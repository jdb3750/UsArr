package httpapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/releases"
)

// A grab has three outcomes and the user-facing wording for each has to say
// exactly what UsArr knows and nothing more. These tests pin which bucket each
// error renders into, so moving one is a failing build.
//
// internal/servarr is deliberately not imported here (doc.go): these cases wrap
// the sentinels internal/releases exports, which is the whole surface this
// package is allowed to see.

func TestGrabErrorClassifiesTheThreeOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want ErrorCode
	}{
		// SENT, OUTCOME UNKNOWN — the only bucket that declines to assert.
		{"dispatched with no usable answer", releases.ErrGrabOutcomeUnknown, CodeGrabOutcomeUnknown},

		// DEFINITELY NOT SENT — every one of these keeps asserting failure,
		// because for every one of them UsArr genuinely knows.
		{"no enabled download client", releases.ErrNoDownloadClient, CodeNoDownloadClient},
		{"a body Prowlarr would not bind", releases.ErrRequestRejected, CodeGrabFailed},
		{"the candidate expired", releases.ErrCandidateExpired, CodeExpired},
		{"the indexer withdrew the release", releases.ErrGrabCacheMiss, CodeNoLongerOffered},
		{"the recovery search did not complete", releases.ErrReSearchFailed, CodeSearchFailed},
		{"no such candidate", releases.ErrCandidateNotFound, CodeNotFound},
		{"out of the caller's scope", releases.ErrForbidden, CodeNotFound},
		{"anything unclassified", fmt.Errorf("some new failure nobody has sorted yet"), CodeGrabFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := grabError(fmt.Errorf("grabbing: %w", tc.err))
			if got.Code != tc.want {
				t.Fatalf("code = %q, want %q", got.Code, tc.want)
			}
		})
	}
}

// The regression that would reintroduce the harm in the other direction.
//
// Nothing was sent for any of these. Telling the user it might be downloading
// sends them hunting through a download client for something that is not there,
// and — worse — it makes the one message that means "this may really be running"
// indistinguishable from ordinary noise.
func TestGrabErrorNeverRendersAnUndispatchedGrabAsSentUnknown(t *testing.T) {
	notSent := []error{
		releases.ErrRequestRejected,
		releases.ErrNoDownloadClient,
		releases.ErrCandidateExpired,
		releases.ErrCandidateNotFound,
		releases.ErrForbidden,
		releases.ErrGrabCacheMiss,
		releases.ErrReSearchFailed,
		releases.ErrNotConfigured,
		fmt.Errorf("decoding stored release for candidate 4: unexpected end of JSON input"),
	}
	for _, err := range notSent {
		got := grabError(fmt.Errorf("grabbing: %w", err))
		if got.Code == CodeGrabOutcomeUnknown {
			t.Errorf("%v renders as sent-unknown; nothing was dispatched for it", err)
		}
		if strings.Contains(got.Message, "may or may not have been added") {
			t.Errorf("%v carries the sent-unknown wording: %q", err, got.Message)
		}
	}
}

// The sent-unknown wording, in full. Every clause here is load-bearing.
func TestGrabUnknownWordingAssertsNothingAndPointsSomewhereUseful(t *testing.T) {
	msg := grabUnknownMessage("Label plugin is not enabled or the label does not exist")

	// §17.5's two required clauses, in its own words. The frontend renders
	// against that section, so these are a spec check and not a style check.
	if !strings.Contains(msg, "sent this release to Prowlarr") {
		t.Errorf("the message does not say the release was sent: %q", msg)
	}
	if !strings.Contains(msg, "the download client reported an error") {
		t.Errorf("the message does not say the download client reported an error: %q", msg)
	}
	if !strings.Contains(msg, "may or may not have been added") {
		t.Errorf("the message does not say the outcome is unknown, in §17.5's words: %q", msg)
	}
	// "Sent" is the strongest true word §17.5 allows for a handed-over state.
	for _, overclaim := range []string{"downloading now", "succeeded", "the download started"} {
		if strings.Contains(msg, overclaim) {
			t.Errorf("the message overclaims (%q): %q", overclaim, msg)
		}
	}
	// It must not claim either outcome. "failed", "did not go through" and
	// "downloaded" are all assertions this message is not entitled to make.
	for _, forbidden := range []string{"did not go through", "the grab failed", "was not sent"} {
		if strings.Contains(msg, forbidden) {
			t.Errorf("the message asserts an outcome it cannot know (%q): %q", forbidden, msg)
		}
	}
	// It points at the download client, which is the only place the truth is.
	if !strings.Contains(msg, "Check your download client") {
		t.Errorf("the message does not point at the download client: %q", msg)
	}
	// It warns about the actual cost of getting this wrong.
	if !strings.Contains(msg, "twice") {
		t.Errorf("the message does not warn about grabbing the release twice: %q", msg)
	}
	// It quotes Prowlarr rather than paraphrasing it.
	if !strings.Contains(msg, "Label plugin is not enabled or the label does not exist") {
		t.Errorf("Prowlarr's own message was dropped: %q", msg)
	}
	// And it names the remedy for the common case without requiring the reader
	// to know what a label plugin is. Prowlarr pre-fills Deluge's Default
	// Category with "prowlarr" and Deluge rejects a label nobody created, so a
	// first grab on default settings lands exactly here.
	if !strings.Contains(msg, "Settings → Download Clients") {
		t.Errorf("the message does not name where the category is configured: %q", msg)
	}

	// With no answer at all there is nothing to quote, and the message must not
	// invent one or dangle a reference to a message that is not there.
	bare := grabUnknownMessage("")
	if strings.Contains(bare, "Prowlarr said") || strings.Contains(bare, "message below") {
		t.Errorf("the no-answer wording refers to a message that does not exist: %q", bare)
	}
	if !strings.Contains(bare, "never sent an answer back") {
		t.Errorf("the no-answer wording does not say so: %q", bare)
	}
}

// "Test connection" is wrong for every one of these, and it is the specific
// wrong answer this whole change exists to stop giving.
//
// A 500 from the label step is not a connectivity problem: the connection
// worked, and Prowlarr's own Test button goes green in exactly this
// configuration because it skips the label check when no category mappings are
// set. Offering to test the connection sends the user to the one component that
// is provably fine, and it comes back green.
func TestGrabErrorOffersNoConnectionTestWhereTheConnectionIsFine(t *testing.T) {
	for _, err := range []error{
		releases.ErrGrabOutcomeUnknown,
		releases.ErrRequestRejected,
		releases.ErrReSearchFailed,
		releases.ErrNoDownloadClient,
	} {
		got := grabError(fmt.Errorf("grabbing: %w", err))
		if got.Action == "Test connection" {
			t.Errorf("%v offers %q; the connection is not the problem", err, got.Action)
		}
	}

	// The sent-unknown case points at the download client instead.
	if got := grabError(fmt.Errorf("x: %w", releases.ErrGrabOutcomeUnknown)); got.Action != "Check your download client" {
		t.Errorf("action = %q, want the download client", got.Action)
	}
}

// The re-search failure used to borrow no_longer_offered's message and tell the
// user the indexer had withdrawn the release. Nothing was sent either way, so
// the verdict was right; the cause was invented.
func TestFailedReSearchDoesNotBlameTheIndexer(t *testing.T) {
	got := grabError(fmt.Errorf("grabbing: %w", releases.ErrReSearchFailed))
	if got.Code == CodeNoLongerOffered {
		t.Fatal("a search that never completed is no evidence about what the indexer offers")
	}
	if strings.Contains(got.Message, "no longer offers") {
		t.Errorf("the message still blames the indexer: %q", got.Message)
	}
	if !strings.Contains(got.Message, "Nothing was sent to your download client") {
		t.Errorf("the message does not say nothing was dispatched: %q", got.Message)
	}
}
