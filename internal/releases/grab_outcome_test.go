package releases

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/servarr"
	"github.com/jdb3750/UsArr/internal/store"
)

// A grab has THREE outcomes, not two, and the tests in this file exist to make
// an error changing bucket a failing build rather than a silent regression.
//
//	definitely not sent  — assert failure; UsArr knows.
//	sent, unknown        — assert nothing; Prowlarr adds before it configures
//	                       and does not roll back, so a 500 may well be a
//	                       running download.
//	sent and confirmed   — including a 2xx whose body would not parse.
//
// Getting the first two the wrong way round is the harm in both directions: a
// false failure invites a duplicate grab on a private tracker, and a false
// "might be downloading" on something that never left the process teaches the
// user to ignore the message that means it.

// grabOutcome is what one scripted grab produced: the result, the error, and the
// fake store, so every case below reads as one line.
type grabOutcome struct {
	res   GrabResult
	err   error
	store *fakeStore
}

// grabbing runs one grab against a scripted client error.
func grabbing(t *testing.T, clientErr error) grabOutcome {
	t.Helper()
	now := time.Unix(1_755_000_000, 0).UTC()
	st := newFakeStore()
	rel := release("g1", "Dune.Part.Two.2024.2160p", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	hash := "abc123def456"
	rel.InfoHash = &hash
	rel.DownloadClientID = servarr.Int32(2)
	id := storedCandidate(t, st, rel, now)

	c := &fakeClient{clients: enabledClients()}
	if clientErr != nil {
		c.grabResponses = []error{clientErr}
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: st,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	res, err := svc.Grab(context.Background(), ownerScope, id)
	return grabOutcome{res: res, err: err, store: st}
}

// SENT, OUTCOME UNKNOWN.
//
// Every error here is one where the POST provably left the process and the
// answer says nothing about what the download client did with it. The real
// report that prompted this: Prowlarr added the torrent to Deluge, threw on the
// label step, returned a bare 500, and UsArr told the owner it had failed.
func TestGrabReportsADispatchedGrabWithNoAnswerAsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{
		name: "a 500 with Prowlarr's own message",
		err: &servarr.APIError{
			Op: "Grab", Method: "POST", Path: "/api/v1/search", Status: 500,
			Err:     servarr.ErrServer,
			Message: "Label plugin is not enabled or the label does not exist",
		},
	}, {
		// A 5xx with no body at all is the shape the owner actually saw.
		name: "a bare 500",
		err: &servarr.APIError{
			Op: "Grab", Method: "POST", Path: "/api/v1/search", Status: 500,
			Err: servarr.ErrServer,
		},
	}, {
		name: "a timeout after the request went out",
		err:  &servarr.APIError{Op: "Grab", Message: "deadline exceeded", Err: servarr.ErrTimeout},
	}, {
		name: "the caller giving up on a request already on the wire",
		err:  &servarr.APIError{Op: "Grab", Message: "canceled", Err: context.Canceled},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got := grabbing(t, tc.err)

			if !errors.Is(got.err, ErrGrabOutcomeUnknown) {
				t.Fatalf("err = %v, want ErrGrabOutcomeUnknown\n"+
					"this error dispatched the release; reporting it as a failure invites the "+
					"user to grab the same release twice", got.err)
			}
			// The upstream sentinel survives the wrap, so a caller that wants to
			// know WHICH kind of unknown still can.
			if !errors.Is(got.err, tc.err) {
				t.Errorf("the underlying error was dropped: %v", got.err)
			}

			// THE JOIN KEY. A torrent on disk with no provenance row can never be
			// attached to the grab that produced it once library sync lands, and
			// download_id — the infohash — is the only key an importer supplies.
			if len(got.store.provenance) != 1 {
				t.Fatalf("got %d provenance rows, want 1: without one, the infohash of a "+
					"download that may well be running is lost permanently", len(got.store.provenance))
			}
			p := got.store.provenance[0]
			if p.DownloadID != "abc123def456" || p.TorrentInfoHash != "abc123def456" {
				t.Errorf("downloadId/infoHash = %q/%q; the join key is the whole reason this row exists",
					p.DownloadID, p.TorrentInfoHash)
			}

			// AND IT MUST NOT BE MISTAKABLE FOR A CONFIRMED GRAB. provenance has
			// no back-reference to audit_log, so this column is the only thing a
			// reader starting from provenance — the import join, the recent-grabs
			// block — has to go on.
			if p.AcquisitionState != AcquisitionUnconfirmed {
				t.Errorf("acquisition_state = %q, want %q: a row that reads as confirmed makes the "+
					"import join attach history UsArr never verified, which is worse than no row at all",
					p.AcquisitionState, AcquisitionUnconfirmed)
			}
			// Confidence is NOT the discriminator: it means match confidence and
			// is gated at >= 1.0 by a partial index, so demoting it would hide a
			// perfectly-identified row from the reads that want it.
			if p.Confidence != 1.0 {
				t.Errorf("confidence = %v, want 1.0; unconfirmed is not the same as unidentified", p.Confidence)
			}

			// The caller gets the row id even though it also gets an error, so
			// the audit row can name it.
			if got.res.ProvenanceID != got.store.provenance[0].ID {
				t.Errorf("ProvenanceID = %d, want the row that was just written (%d)",
					got.res.ProvenanceID, got.store.provenance[0].ID)
			}
		})
	}
}

// DEFINITELY NOT SENT.
//
// This is the regression guard in the other direction. Every error here is one
// UsArr genuinely knows never reached the download client, so it must keep
// asserting failure. If one of them starts rendering as "it might be
// downloading", the message stops meaning anything on the occasions it is true.
func TestGrabNeverReportsAnUndispatchedGrabAsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{{
		// SendReportToClient's FIRST statement, before anything is added.
		name: "no enabled download client",
		want: ErrNoDownloadClient,
		err: &servarr.APIError{
			Op: "Grab", Status: 500, Err: servarr.ErrNoDownloadClient,
			Message: "Torrent Download client isn't configured yet",
		},
	}, {
		// The body is built server-side; a 400 is UsArr's own defect.
		name: "a request Prowlarr would not bind",
		want: ErrRequestRejected,
		err: &servarr.APIError{
			Op: "Grab", Status: 400, Err: servarr.ErrValidation,
			Message: "One or more validation errors occurred.",
		},
	}, {
		// Never left the process at all.
		name: "the circuit breaker refusing the call",
		err:  &servarr.APIError{Op: "Grab", Err: servarr.ErrBreakerOpen},
	}, {
		// An SSRF-policy refusal, or a body that would not marshal.
		name: "a client-side rejection",
		err:  &servarr.APIError{Op: "Grab", Err: servarr.ErrInvalidRequest},
	}, {
		name: "a bad API key",
		err:  &servarr.APIError{Op: "Grab", Status: 401, Err: servarr.ErrUnauthorized},
	}, {
		// 409 is ReleaseDownloadException, raised fetching from the INDEXER —
		// so it too means nothing reached the client.
		name: "the indexer refusing to hand over the release",
		err:  &servarr.APIError{Op: "Grab", Status: 409, Err: servarr.ErrConflict},
	}, {
		name: "a status nobody has classified",
		err:  &servarr.APIError{Op: "Grab", Status: 418, Err: servarr.ErrUnexpectedStatus},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got := grabbing(t, tc.err)

			if got.err == nil {
				t.Fatal("a grab that never dispatched must not report success")
			}
			if errors.Is(got.err, ErrGrabOutcomeUnknown) {
				t.Fatalf("err = %v, classified as sent-unknown\n"+
					"nothing was sent for this error, and saying otherwise sends the user to hunt "+
					"through a download client for something that is not there", got.err)
			}
			if tc.want != nil && !errors.Is(got.err, tc.want) {
				t.Errorf("err = %v, want %v", got.err, tc.want)
			}
			// No dispatch, no provenance. An unconfirmed row here would be a
			// join key for a download that does not exist.
			if len(got.store.provenance) != 0 {
				t.Errorf("wrote %d provenance rows for a grab that never left the process",
					len(got.store.provenance))
			}
		})
	}
}

// SENT AND CONFIRMED.
//
// servarr.ErrDecode is produced strictly AFTER Client.do's non-2xx branch has
// returned — verified in internal/servarr/client.go, where the `resp.StatusCode
// < 200 || >= 300` block returns parseErrorBody's *APIError before any decode of
// the body is attempted. So it can only follow a 2xx: Prowlarr confirmed the
// grab and UsArr failed to parse its own receipt. Reporting that as a failure
// was a second, independent false failure.
func TestGrabTreatsAnUnreadableConfirmationAsSuccess(t *testing.T) {
	got := grabbing(t, &servarr.APIError{
		Op: "Grab", Method: "POST", Path: "/api/v1/search", Status: 200,
		Message: "decoding response: unexpected end of JSON input",
		Err:     servarr.ErrDecode,
	})
	if got.err != nil {
		t.Fatalf("Grab: %v\n"+
			"a 2xx IS Prowlarr's confirmation — there is no download id or queue id to poll — so "+
			"a body UsArr could not parse is a receipt we failed to read, not a failed grab", got.err)
	}
	if !got.res.ResponseUnreadable {
		t.Error("ResponseUnreadable is false; the caller cannot say so if it is not told")
	}
	if len(got.store.provenance) != 1 {
		t.Fatalf("got %d provenance rows, want 1", len(got.store.provenance))
	}
	if state := got.store.provenance[0].AcquisitionState; state != AcquisitionConfirmed {
		t.Errorf("acquisition_state = %q, want %q: Prowlarr confirmed this one", state, AcquisitionConfirmed)
	}
	// The title still comes out right: the response was unreadable, so it falls
	// back to the stored release, which is where the grab body came from anyway.
	if got.res.ReleaseTitle != "Dune.Part.Two.2024.2160p" {
		t.Errorf("ReleaseTitle = %q; it must fall back to the stored release", got.res.ReleaseTitle)
	}
}

// A re-search that did not COMPLETE is not evidence that the indexer withdrew
// the release. Nothing was sent either way — the grab-cache 404 precedes
// dispatch — so this is still a failure; it is the CAUSE that was wrong.
func TestGrabSeparatesAFailedReSearchFromAWithdrawnRelease(t *testing.T) {
	now := time.Unix(1_755_000_000, 0).UTC()
	st := newFakeStore()
	rel := release("g1", "A", 1, "Alpha", servarr.ProtocolTorrent, 2000)
	id := storedCandidate(t, st, rel, now)

	c := &fakeClient{
		clients:       enabledClients(),
		grabResponses: []error{&servarr.APIError{Op: "Grab", Status: 404, Err: servarr.ErrNotFound}},
		// The re-search itself falls over.
		searchErrs: map[int32]error{1: &servarr.APIError{Op: "Search", Err: servarr.ErrTimeout}},
	}
	svc, err := NewService(Config{
		InstanceID: testInstanceID, Client: c, Store: st,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Grab(context.Background(), ownerScope, id)
	if !errors.Is(err, ErrReSearchFailed) {
		t.Fatalf("err = %v, want ErrReSearchFailed", err)
	}
	if errors.Is(err, ErrGrabCacheMiss) {
		t.Error("a failed re-search must not claim the indexer no longer offers the release; " +
			"the search never got an answer, so it is no evidence about the indexer at all")
	}
	if errors.Is(err, ErrGrabOutcomeUnknown) {
		t.Error("nothing was dispatched: the grab-cache 404 precedes SendReportToClient")
	}
	if len(st.provenance) != 0 {
		t.Errorf("wrote %d provenance rows for a grab that never dispatched", len(st.provenance))
	}
}

// The two acquisition-state vocabularies are the wire format of one TEXT column
// with no CHECK constraint behind it, spelled in two packages that deliberately
// do not share a type. Drift would be silent: rows would be written with a value
// no reader recognises, and every one of them would read as "not confirmed".
func TestAcquisitionStatesMatchTheStore(t *testing.T) {
	if AcquisitionConfirmed != store.AcquisitionConfirmed {
		t.Errorf("confirmed: releases has %q, store has %q", AcquisitionConfirmed, store.AcquisitionConfirmed)
	}
	if AcquisitionUnconfirmed != store.AcquisitionUnconfirmed {
		t.Errorf("unconfirmed: releases has %q, store has %q", AcquisitionUnconfirmed, store.AcquisitionUnconfirmed)
	}
}

// UpstreamMessage exists so internal/httpapi can quote Prowlarr without
// importing internal/servarr. What it must never leak is Description, which is a
// full .NET stack trace.
func TestUpstreamMessageQuotesTheServiceAndNotItsStackTrace(t *testing.T) {
	err := &servarr.APIError{
		Op: "Grab", Status: 500, Err: servarr.ErrServer,
		Message:     "Label plugin is not enabled",
		Description: "System.InvalidOperationException at NzbDrone.Core.Download…",
	}
	got := UpstreamMessage(errors.Join(ErrGrabOutcomeUnknown, err))
	if got != "Label plugin is not enabled" {
		t.Errorf("UpstreamMessage = %q, want Prowlarr's own message", got)
	}
	if strings.Contains(got, "System.") {
		t.Error("the .NET stack trace escaped into a user-facing string")
	}

	// A transport failure carries no message FROM the service; attributing
	// UsArr's own "deadline exceeded" to Prowlarr would put words in its mouth.
	if got := UpstreamMessage(&servarr.APIError{Op: "Grab", Message: "deadline exceeded", Err: servarr.ErrTimeout}); got != "" {
		t.Errorf("UpstreamMessage = %q for a transport error, want empty", got)
	}
}
