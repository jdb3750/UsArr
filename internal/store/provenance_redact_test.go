package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/servarr"
)

// The repair pass for credentials already on disk.
//
// The redactor under test is servarr.RedactURL — the SAME function the grab path
// calls — imported deliberately rather than faked. A fake would let this test
// pass while the stored and served values disagreed about what a secret is,
// which is the one property the pass exists to guarantee.

// seedProvenanceURL writes a row with nzb_info_url set verbatim, bypassing
// InsertProvenance's own handling, because the rows this pass repairs were
// written by a build that did not redact. Going through the current write path
// would seed already-clean rows and assert nothing.
func seedProvenanceURL(t *testing.T, s *Store, title, url string) int64 {
	t.Helper()
	var id int64
	err := s.write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO provenance (protocol, release_title, source_system, nzb_info_url)
			VALUES ('torrent', ?, 'prowlarr', ?)`, title, nullString(url))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		t.Fatalf("seed provenance %q: %v", title, err)
	}
	return id
}

func storedURL(t *testing.T, s *Store, id int64) string {
	t.Helper()
	var got sql.NullString
	if err := s.db.Read().QueryRowContext(t.Context(),
		`SELECT nzb_info_url FROM provenance WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatalf("read provenance %d: %v", id, err)
	}
	return got.String
}

func TestRedactStoredProvenanceURLs(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	// Both leak shapes, because both are real and the deny-list only ever
	// covered the first.
	const queryPasskey = "0123456789abcdef0123456789abcdef"
	const pathPasskey = "fedcba9876543210fedcba9876543210"

	queryRow := seedProvenanceURL(t, s, "Query.Passkey.Release-GROUP",
		"https://tracker.example/details.php?id=42&passkey="+queryPasskey)
	pathRow := seedProvenanceURL(t, s, "Path.Passkey.Release-GROUP",
		"https://tracker.example/rss/"+pathPasskey+"/torrents")

	// Rows that carry no credential must come out byte-identical. A repair pass
	// that rewrites clean rows is corrupting permanent data to no purpose.
	cleanURL := "https://tracker.example/details/12345"
	cleanRow := seedProvenanceURL(t, s, "Clean.Release-GROUP", cleanURL)
	emptyRow := seedProvenanceURL(t, s, "No.URL.Release-GROUP", "")

	changed, err := s.RedactStoredProvenanceURLs(ctx, servarr.RedactURL)
	if err != nil {
		t.Fatalf("RedactStoredProvenanceURLs: %v", err)
	}
	if changed != 2 {
		t.Fatalf("changed %d rows, want exactly the 2 that carried a credential", changed)
	}

	for _, tc := range []struct {
		name   string
		id     int64
		secret string
	}{
		{"query parameter", queryRow, queryPasskey},
		{"path segment", pathRow, pathPasskey},
	} {
		got := storedURL(t, s, tc.id)
		if strings.Contains(got, tc.secret) {
			t.Errorf("%s passkey survived the repair pass: %q\n"+
				"provenance rows are permanent, so this credential would be in every backup "+
				"taken from here on, forever", tc.name, got)
		}
		if !strings.Contains(got, "tracker.example") {
			t.Errorf("%s: the repair pass lost the host: %q\n"+
				"which tracker a grab came from is what this column exists to record", tc.name, got)
		}
	}

	if got := storedURL(t, s, cleanRow); got != cleanURL {
		t.Errorf("a row with no credential was rewritten: %q -> %q", cleanURL, got)
	}
	if got := storedURL(t, s, emptyRow); got != "" {
		t.Errorf("a row with no URL was given one: %q", got)
	}
}

// TestRedactStoredProvenanceURLsIsIdempotent is what lets the pass run on every
// start rather than behind a run-once gate — and a run-once gate is the wrong
// shape for a security repair, because a process killed mid-pass would leave the
// rest unredacted with the gate already satisfied.
func TestRedactStoredProvenanceURLsIsIdempotent(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	id := seedProvenanceURL(t, s, "Release-GROUP",
		"https://tracker.example/rss/0123456789abcdef0123456789abcdef/torrents?passkey=fedcba9876543210fedcba9876543210")

	first, err := s.RedactStoredProvenanceURLs(ctx, servarr.RedactURL)
	if err != nil {
		t.Fatal(err)
	}
	if first != 1 {
		t.Fatalf("first pass changed %d rows, want 1", first)
	}
	after := storedURL(t, s, id)

	second, err := s.RedactStoredProvenanceURLs(ctx, servarr.RedactURL)
	if err != nil {
		t.Fatal(err)
	}
	if second != 0 {
		t.Errorf("second pass changed %d rows; redacting an already-redacted URL must be a no-op", second)
	}
	if got := storedURL(t, s, id); got != after {
		t.Errorf("the second pass altered the value: %q -> %q", after, got)
	}
}

// The pass must page rather than assume one batch. redactBatchSize rows exactly
// is the boundary where a "fewer than a full batch means we are done" check gets
// it wrong if the loop is written the obvious way.
func TestRedactStoredProvenanceURLsPagesPastOneBatch(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t)

	const n = redactBatchSize + 7
	ids := make([]int64, 0, n)
	for i := range n {
		ids = append(ids, seedProvenanceURL(t, s, "Release-GROUP",
			"https://tracker.example/rss/0123456789abcdef0123456789abcdef/x?i="+strconv.Itoa(i)))
	}

	changed, err := s.RedactStoredProvenanceURLs(ctx, servarr.RedactURL)
	if err != nil {
		t.Fatal(err)
	}
	if changed != int64(n) {
		t.Fatalf("changed %d rows, want %d — the pass stopped at a batch boundary", changed, n)
	}
	// Spot-check the far end, which is the half a single-batch pass misses.
	if got := storedURL(t, s, ids[n-1]); strings.Contains(got, "0123456789abcdef0123456789abcdef") {
		t.Errorf("the last row was never repaired: %q", got)
	}
}

func TestRedactStoredProvenanceURLsNeedsARedactor(t *testing.T) {
	if _, err := newTestStore(t).RedactStoredProvenanceURLs(t.Context(), nil); err == nil {
		t.Error("a nil redactor was accepted; the pass would silently repair nothing")
	}
}
