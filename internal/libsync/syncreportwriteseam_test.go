package libsync

import (
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// THE WRITE SEAM ON sync_report.detail.
//
// Nine statements in three packages write this column, each marshalling its own
// struct or map. Several of them carry a string the UPSTREAM chose — a container
// name, a decline reason, an external identifier value — and
// reference/security.md §5 keeps upstream response text with credentials in it
// out of the column, because rows from it are lifted onto GET /api/v1/libraries.
// The redaction is applied at each of those assignments.
//
// This test guards the SEAM rather than the list: it enumerates no writer, so a
// tenth statement added tomorrow with no redaction fails here without anyone
// having remembered to update anything.
//
// ⚠️ IT IS DRIVEN FROM A FAKE Source AND NOT THROUGH AN HTTP CLIENT, AND THAT IS
// THE WHOLE REASON IT CAN FIRE. internal/bookorbit passes every piece of upstream
// prose through ssrf.RedactText at its own client boundary
// (internal/bookorbit/catalogue.go's text → clean), so an end-to-end test driven
// by the BookOrbit fake finds a clean column whether or not any writer redacts —
// measured, by deleting a redaction and watching such a test stay green.
// internal/kavita does NOT redact a library name on the way out
// (internal/kavita/redact.go's LibraryView), so the writers are load-bearing;
// a fake Source is how the poison reaches them.

const writeSeamSecret = "s3cr3t-write-seam-token"

// writeSeamPoison is the URL shape ssrf.RedactText is specified to close: an
// http/https substring carrying a credential-named query parameter.
//
// ⚠️ WHAT A GREEN HERE DOES NOT MEAN, said plainly so nobody reads more into it.
// RedactText's own doc is explicit that a bare secret NOT inside a URL passes
// through untouched, so this asserts the URL-shaped case and only that. A writer
// that copied an upstream string carrying a naked token would still pass. Closing
// that needs a different mechanism than redaction, and §5 does not ask for one.
const writeSeamPoison = "https://books.example/scan?apikey=" + writeSeamSecret

func TestNoSyncReportWriterLeavesUpstreamTextUnredacted(t *testing.T) {
	s := newTestStore(t)
	inst := fixtureInstance(t, s)

	// Every upstream-chosen string this fixture can reach a writer with is
	// poisoned: two container names, a decline reason, and an external
	// identifier value that two items claim at full confidence — which is the
	// collision that produces an identity_conflict row.
	src := &fakeSource{
		containers: []store.CatalogueContainer{
			{RemoteID: "1", Name: "Fiction " + writeSeamPoison, Kind: "book"},
			{
				RemoteID: "2", Name: "Refused " + writeSeamPoison,
				DeclineReason: "the upstream said " + writeSeamPoison,
			},
		},
		items: []store.CatalogueItem{
			{
				RemoteID: "10", RemoteKind: "series", ContainerID: "1", Kind: "book",
				Title: "The Hobbit", SortTitle: "Hobbit, The", NormalizedTitle: "hobbit the",
				ExternalIDs: []store.ExternalIdentifier{
					{Source: "weblink", Value: writeSeamPoison, Confidence: 1},
				},
			},
			{
				RemoteID: "11", RemoteKind: "series", ContainerID: "1", Kind: "book",
				Title: "Dune", SortTitle: "Dune", NormalizedTitle: "dune",
			},
		},
	}

	im := newImporter(t, s, src)
	if _, err := im.FullImport(t.Context(), inst); err != nil {
		t.Fatalf("first FullImport: %v", err)
	}

	// A SECOND IMPORT, because the identity_conflict writer needs one. An
	// existing service_item_link pins which work a remote item is, so the
	// collision only happens once item 11 is already a work of its own and THEN
	// claims the identifier item 10 holds — which is exactly the shape
	// internal/store's own conflict test builds, and it takes two passes.
	src.items[1].ExternalIDs = []store.ExternalIdentifier{
		{Source: "weblink", Value: writeSeamPoison, Confidence: 1},
	}
	if _, err := im.FullImport(t.Context(), inst); err != nil {
		t.Fatalf("second FullImport: %v", err)
	}

	rows, err := s.DB().Read().QueryContext(t.Context(),
		`SELECT id, kind, COALESCE(remote_id, ''), detail FROM sync_report ORDER BY id`)
	if err != nil {
		t.Fatalf("read sync_report: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var scanned, redacted int
	kinds := map[string]bool{}
	for rows.Next() {
		var id int64
		var kind, remoteID, detail string
		if err := rows.Scan(&id, &kind, &remoteID, &detail); err != nil {
			t.Fatalf("scan sync_report: %v", err)
		}
		scanned++
		kinds[kind] = true
		if strings.Contains(detail, writeSeamSecret) {
			t.Errorf("sync_report row %d (kind %q, remote_id %q) carries an upstream "+
				"credential in its detail.\n  detail: %s\n"+
				"reference/security.md §5 keeps upstream response text out of this column, "+
				"and rows from it are lifted onto GET /api/v1/libraries. The fix is "+
				"ssrf.RedactText at the STRUCT ASSIGNMENT that produced this string — not on "+
				"the map it is marshalled through, and not on the read. The struct value "+
				"usually escapes to a log line as well, so a redaction on the map guards one "+
				"exit of two; and redacting on the read makes the value shown differ from the "+
				"value stored, which is the failure recordContainerObservation's comment in "+
				"internal/store/catalogue.go works through at length. That function is the "+
				"shape to copy.", id, kind, remoteID, detail)
		}
		if strings.Contains(detail, "apikey=REDACTED") {
			redacted++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sync_report: %v", err)
	}

	// ⚠️ THE NON-VACUITY CHECKS, AND THEY ARE WHAT MAKES THE GREEN MEAN ANYTHING.
	// A scan over no rows passes; so does a scan over rows the poison never
	// reached. Either would be an instrument reporting clean because it looked at
	// nothing — which is the failure the guard replacement in this same landing
	// exists to stop, and it is no better for being one file away.
	if scanned == 0 {
		t.Fatal("no sync_report rows at all, so the scan above asserted nothing")
	}
	if len(kinds) < 3 {
		t.Errorf("the import produced only %d kinds of sync_report row (%v); this guard "+
			"sweeps several writers at once and is nearly vacuous over one", len(kinds), kinds)
	}
	if redacted == 0 {
		t.Fatalf("no sync_report row carries `apikey=REDACTED`, over %d rows of kinds %v. "+
			"The poison never reached the column, so a pass says nothing about redaction — "+
			"fix the fixture, not the assertion", scanned, kinds)
	}
	t.Logf("write seam: %d sync_report rows over %d kinds, %d carrying a redacted URL",
		scanned, len(kinds), redacted)
}
