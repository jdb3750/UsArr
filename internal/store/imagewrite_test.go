package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"
)

// seedPosterFixture puts one instance and two live works in place, so the writer
// under test has something real to link to and something real to fail against.
func seedPosterFixture(t *testing.T, s *Store) {
	t.Helper()
	stmts := []string{
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (7, 'bookorbit', 'library', 'BookOrbit', 'http://books.example', X'00')`,
		`INSERT INTO work (id, kind, title, sort_title, normalized_title)
		   VALUES (1, 'book', 'Dune', 'dune', 'dune')`,
		`INSERT INTO work (id, kind, title, sort_title, normalized_title)
		   VALUES (2, 'book', 'Emma', 'emma', 'emma')`,
		`INSERT INTO work (id, kind, title, sort_title, normalized_title, deleted_at)
		   VALUES (3, 'book', 'A Tombstone', 'a tombstone', 'a tombstone', '2026-08-16 09:00:00')`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (7, 1, 'b1', 'book', '2026-08-16 09:00:00')`,
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (7, 2, 'b2', 'book', '2026-08-16 09:00:00')`,
		// A LIVE link onto the TOMBSTONED work, so "the link resolves and the
		// work is gone" is a reachable case and not a hypothetical.
		`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
		   VALUES (7, 3, 'b3', 'book', '2026-08-16 09:00:00')`,
	}
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, stmt := range stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("%s: %w", stmt, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}

// validPoster is the row every rejection case below mutates one field of, so a
// refusal can only ever be attributed to that field.
//
// The cache key is mostly zeroes, on fixtureAPIKey's convention: sixteen
// plausible-looking hex characters assigned to something named `key` is what
// `make check`'s gitleaks pass reports as a generic-api-key leak — measured, not
// guessed — and a waiver here would be a waiver the next fixture inherits.
func validPoster() PosterAsset {
	return PosterAsset{
		RemoteKind:        "book",
		RemoteID:          "b1",
		SourceURL:         "http://books.example/api/v1/books/11/cover",
		CacheKey:          "00000000000000ab",
		Format:            ImageFormatJPEG,
		OriginClass:       ImageOriginConfigured,
		ServiceInstanceID: 7,
		Width:             800,
		Height:            1200,
		FetchedAt:         testNow,
	}
}

// TestPutPosterAssetWritesTheRowAndLinksTheWork asserts every column the writer
// claims to set, and then asserts the thing that actually matters: the SERVING
// path can now find it.
//
// It fails against an empty implementation on each of those independently — a
// writer that inserted the asset and skipped the UPDATE passes the column checks
// and fails the lookup, which is exactly the split the two halves exist to
// separate.
func TestPutPosterAssetWritesTheRowAndLinksTheWork(t *testing.T) {
	s := newTestStore(t)
	seedPosterFixture(t, s)

	p := validPoster()
	id, err := s.PutPosterAsset(t.Context(), p)
	if err != nil {
		t.Fatalf("PutPosterAsset: %v", err)
	}
	if id <= 0 {
		t.Fatalf("PutPosterAsset returned asset id %d", id)
	}

	var (
		src, class, role, key, format, state, fetched string
		instance                                      sql.NullInt64
		w, h                                          int
	)
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT source_url, origin_class, origin_service_instance_id, role,
		        width, height, cache_key, format, state, fetched_at
		   FROM image_asset WHERE id = ?`, id,
	).Scan(&src, &class, &instance, &role, &w, &h, &key, &format, &state, &fetched); err != nil {
		t.Fatalf("reading back image_asset %d: %v", id, err)
	}

	for _, c := range []struct{ name, got, want string }{
		{"source_url", src, p.SourceURL},
		{"origin_class", class, ImageOriginConfigured},
		{"role", role, ImageRolePoster},
		{"cache_key", key, p.CacheKey},
		{"format", format, ImageFormatJPEG},
		{"state", state, ImageStateReady},
		{"fetched_at", fetched, FormatTime(testNow)},
	} {
		if c.got != c.want {
			t.Errorf("image_asset.%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if !instance.Valid || instance.Int64 != 7 {
		t.Errorf("origin_service_instance_id = %v, want 7", instance)
	}
	if w != 800 || h != 1200 {
		t.Errorf("dimensions = %dx%d, want 800x1200 (the SOURCE's, not a rendition's)", w, h)
	}

	// migration 00008's stated invariant: a non-NULL format is co-extensive with
	// state 'ready'. Asserted rather than assumed, because this writer is the one
	// place both are set at once.
	if state != ImageStateReady || format == "" {
		t.Errorf("state %q with format %q breaks 00008's NULL-format-iff-not-ready invariant", state, format)
	}

	var poster sql.NullInt64
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT poster_asset_id FROM work WHERE id = 1`).Scan(&poster); err != nil {
		t.Fatalf("reading back work.poster_asset_id: %v", err)
	}
	if !poster.Valid || poster.Int64 != id {
		t.Fatalf("work.poster_asset_id = %v, want %d — the asset was written but nothing points at it, "+
			"which under imageassets.go's access scope makes it visible to nobody", poster, id)
	}

	// The end the whole pipeline exists for: the serving read now answers.
	got, ok, err := s.LookupImageAsset(t.Context(), OwnerScope(1), p.CacheKey)
	if err != nil {
		t.Fatalf("LookupImageAsset: %v", err)
	}
	if !ok {
		t.Fatal("LookupImageAsset found nothing for a key this writer just wrote; " +
			"GET /img/{key} would still answer not_found on every install")
	}
	if got.ID != id || got.State != ImageStateReady || got.Format.String != ImageFormatJPEG {
		t.Errorf("LookupImageAsset returned %+v, want id %d, state ready, format jpeg", got, id)
	}
}

// TestPutPosterAssetRejectsACredentialInTheSourceURL is the DRILL for
// security.md §5's ingest assertion and for REVIEW-LOG row S-20.
//
// It fails against a writer that has no such check — every case below is a
// perfectly valid INSERT otherwise, so an unguarded writer returns a row id and
// no error for all of them.
func TestPutPosterAssetRejectsACredentialInTheSourceURL(t *testing.T) {
	s := newTestStore(t)
	seedPosterFixture(t, s)

	// Deliberately NOT a textbook example value: `gitleaks` waives anything
	// matching `.+EXAMPLE$` by rule, so a canonical example key is the one drill
	// value that proves nothing about a secret-scanned tree.
	const drill = "not-a-real-key-just-a-drill"

	for _, tc := range []struct{ name, url string }{
		{"apikey", "http://books.example/cover?apikey=" + drill},
		{"camelCase apiKey", "http://books.example/cover?apiKey=" + drill},
		{"api_key", "http://books.example/cover?api_key=" + drill},
		{"access_token", "http://books.example/cover?access_token=" + drill},
		{"opensubsonic token", "http://books.example/cover?u=joe&t=" + drill + "&s=abc"},
		{"tracker passkey", "http://books.example/cover?passkey=" + drill},
		{"userinfo", "http://joe:" + drill + "@books.example/cover"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := validPoster()
			p.SourceURL = tc.url

			id, err := s.PutPosterAsset(t.Context(), p)
			if err == nil {
				t.Fatalf("PutPosterAsset stored a credentialled source_url as image_asset %d; "+
					"security.md §5 requires the ingest path to REFUSE such a row, and "+
					"cache_key = sha256(source_url)[:16] means the credential would also be "+
					"baked into the cache key", id)
			}
			if !errors.Is(err, ErrCredentialInSourceURL) {
				t.Errorf("refused with %v, which is not ErrCredentialInSourceURL", err)
			}
			if !errors.Is(err, ErrCredentialInSourceURL) || errors.Is(err, ErrInvalidImageAsset) {
				// Kept separate on purpose: "this row is malformed" and "this row
				// carries a secret" are different incidents.
				t.Logf("note: %v", err)
			}

			// The refusal must be TOTAL. A writer that rejected the asset but
			// still touched the work would leave the link pointing at nothing.
			assertNoRowsWritten(t, s)
		})
	}
}

// TestPutPosterAssetDoesNotSilentlyStrip pins the half of S-20 that a sanitiser
// would also pass: the row must be REFUSED, not cleaned up and written.
func TestPutPosterAssetDoesNotSilentlyStrip(t *testing.T) {
	s := newTestStore(t)
	seedPosterFixture(t, s)

	p := validPoster()
	p.SourceURL = "http://books.example/cover?apikey=not-a-real-key-just-a-drill"
	if _, err := s.PutPosterAsset(t.Context(), p); err == nil {
		t.Fatal("PutPosterAsset accepted the row")
	}

	var n int
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT count(*) FROM image_asset WHERE source_url LIKE 'http://books.example/cover%'`).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if n != 0 {
		t.Errorf("a stripped-and-stored row survives the refusal (%d rows); the requirement is an "+
			"ASSERTION, and a sanitiser leaves the bug that produced the URL in place", n)
	}
}

// TestPutPosterAssetRejectsAFormatOutsideTheVocabulary is the drill for the call
// migration 00008 and ADR-0050 are owed.
//
// TestImageWritesValidateTheFormatVocabulary can only see that
// ValidImageFormat is REFERENCED somewhere in the tree; this is what checks it is
// CALLED on the value actually stored. A writer that named the validator in a
// comment and stored anything would pass that guard and fail this.
func TestPutPosterAssetRejectsAFormatOutsideTheVocabulary(t *testing.T) {
	s := newTestStore(t)
	seedPosterFixture(t, s)

	for _, format := range []string{
		"",           // 00008: "" is not the way to say unset; NULL is
		"avif",       // deferred by ADR-0050 with a named reopening condition
		"png",        // plausible, and still not in the vocabulary today
		"image/jpeg", // a media type, not a codec token
		"jpg",        // a file suffix, not a codec token
		"JPEG",       // the vocabulary is lowercase
	} {
		t.Run("format="+format, func(t *testing.T) {
			p := validPoster()
			p.Format = format
			_, err := s.PutPosterAsset(t.Context(), p)
			if err == nil {
				t.Fatalf("PutPosterAsset stored format %q, which ValidImageFormat rejects. "+
					"image_asset.format carries NO CHECK constraint (00008), so Go is the only "+
					"place this vocabulary is enforced — this is the ADR-0039 failure repeating", format)
			}
			if !errors.Is(err, ErrInvalidImageAsset) {
				t.Errorf("refused with %v, want ErrInvalidImageAsset", err)
			}
			assertNoRowsWritten(t, s)
		})
	}
}

func TestPutPosterAssetRejectsTheOtherMalformedRows(t *testing.T) {
	s := newTestStore(t)
	seedPosterFixture(t, s)

	mutate := map[string]func(*PosterAsset){
		"no remote kind":   func(p *PosterAsset) { p.RemoteKind = "" },
		"no remote id":     func(p *PosterAsset) { p.RemoteID = "" },
		"no instance":      func(p *PosterAsset) { p.ServiceInstanceID = 0 },
		"empty source_url": func(p *PosterAsset) { p.SourceURL = "" },
		"short cache key":  func(p *PosterAsset) { p.CacheKey = "00000000000000a" },
		"uppercase key":    func(p *PosterAsset) { p.CacheKey = "00000000000000AB" },
		"bad origin class": func(p *PosterAsset) { p.OriginClass = "harvested" },
		"zero width":       func(p *PosterAsset) { p.Width = 0 },
		"negative height":  func(p *PosterAsset) { p.Height = -1 },
	}
	for name, f := range mutate {
		t.Run(name, func(t *testing.T) {
			p := validPoster()
			f(&p)
			if _, err := s.PutPosterAsset(t.Context(), p); err == nil {
				t.Errorf("PutPosterAsset accepted a row with %s", name)
			}
			assertNoRowsWritten(t, s)
		})
	}
}

// TestPutPosterAssetRefusesToOrphanAnAsset pins the ordering argument in
// PutPosterAsset's header: a work that is not there means NO asset row at all.
//
// It fails against a writer that inserts first and links second without a
// transaction — that one leaves a committed asset behind on both cases here.
func TestPutPosterAssetRefusesToOrphanAnAsset(t *testing.T) {
	s := newTestStore(t)
	seedPosterFixture(t, s)

	for name, remoteID := range map[string]string{
		// No service_item_link at all — an item that was never imported, or one
		// whose link was removed between the fetch and this write.
		"no such link": "b-nonexistent",
		// A LIVE link onto a TOMBSTONED work. This is the case a foreign key
		// alone would not catch: the link resolves, and the work is gone.
		"tombstoned work": "b3",
	} {
		t.Run(name, func(t *testing.T) {
			p := validPoster()
			p.RemoteID = remoteID
			p.SourceURL = fmt.Sprintf("http://books.example/api/v1/books/%s/cover", remoteID)

			_, err := s.PutPosterAsset(t.Context(), p)
			if !errors.Is(err, ErrNoSuchWork) {
				t.Fatalf("PutPosterAsset returned %v, want ErrNoSuchWork", err)
			}
			var n int
			if err := s.DB().Read().QueryRowContext(t.Context(),
				`SELECT count(*) FROM image_asset WHERE source_url = ?`, p.SourceURL).Scan(&n); err != nil {
				t.Fatalf("counting: %v", err)
			}
			if n != 0 {
				t.Errorf("the failed link left %d image_asset row(s) behind; an asset no work points "+
					"at is visible to nobody AND blocks the retry, because source_url is UNIQUE", n)
			}
		})
	}
}

// TestPutPosterAssetUpsertsKeepingTheRowID is the re-import case, and the
// assertion is about the ID rather than about the columns.
//
// source_url is UNIQUE. A writer that deleted and reinserted would give the row
// a new id, and `work.poster_asset_id REFERENCES image_asset(id) ON DELETE SET
// NULL` would blank the poster of every OTHER work sharing that artwork on the
// way past — which is exactly what the second work here checks.
func TestPutPosterAssetUpsertsKeepingTheRowID(t *testing.T) {
	s := newTestStore(t)
	seedPosterFixture(t, s)

	p := validPoster()
	first, err := s.PutPosterAsset(t.Context(), p)
	if err != nil {
		t.Fatalf("first PutPosterAsset: %v", err)
	}

	// A second work sharing the same artwork row.
	second := p
	second.RemoteID = "b2"
	sameRow, err := s.PutPosterAsset(t.Context(), second)
	if err != nil {
		t.Fatalf("second PutPosterAsset: %v", err)
	}
	if sameRow != first {
		t.Fatalf("the same source_url produced asset ids %d then %d; source_url is UNIQUE, so this "+
			"is a delete-and-reinsert and it cascades SET NULL through work.poster_asset_id", first, sameRow)
	}

	// Re-fetch with new dimensions — a re-encode of a cover the upstream replaced.
	p.Width, p.Height = 1000, 1500
	p.FetchedAt = testNow.Add(time.Hour)
	again, err := s.PutPosterAsset(t.Context(), p)
	if err != nil {
		t.Fatalf("re-import PutPosterAsset: %v", err)
	}
	if again != first {
		t.Errorf("re-import moved the asset from id %d to %d", first, again)
	}

	var w, h int
	var fetched string
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT width, height, fetched_at FROM image_asset WHERE id = ?`, first).Scan(&w, &h, &fetched); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if w != 1000 || h != 1500 {
		t.Errorf("the upsert did not refresh the dimensions: %dx%d", w, h)
	}
	if fetched != FormatTime(testNow.Add(time.Hour)) {
		t.Errorf("the upsert did not refresh fetched_at: %q", fetched)
	}

	// Both works still point at it.
	for _, id := range []int64{1, 2} {
		var poster sql.NullInt64
		if err := s.DB().Read().QueryRowContext(t.Context(),
			`SELECT poster_asset_id FROM work WHERE id = ?`, id).Scan(&poster); err != nil {
			t.Fatalf("reading work %d: %v", id, err)
		}
		if !poster.Valid || poster.Int64 != first {
			t.Errorf("work %d.poster_asset_id = %v, want %d", id, poster, first)
		}
	}
}

// assertNoRowsWritten is the "the refusal was total" check every rejection case
// shares. A guard that refuses the asset but has already touched `work` is worse
// than no guard: the link then points at a row that was rolled back.
func assertNoRowsWritten(t *testing.T, s *Store) {
	t.Helper()
	var assets, linked int
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT (SELECT count(*) FROM image_asset),
		        (SELECT count(*) FROM work WHERE poster_asset_id IS NOT NULL)`,
	).Scan(&assets, &linked); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if assets != 0 || linked != 0 {
		t.Errorf("a refused write left %d image_asset row(s) and %d linked work(s)", assets, linked)
	}
}

// TestPutPosterAssetResolvesTheWorkInsideItsOwnTransaction is the structural
// half of the CreditSet rule, and it is the reason PosterAsset carries no work
// id at all.
//
// The hazard it forecloses: a caller resolves work 1, then spends a network
// round trip fetching the cover, and in that window work 1 is deleted and its
// `INTEGER PRIMARY KEY` rowid is reused. A writer that took a work id would put
// this cover on whatever now holds it — a cover that silently does not match its
// title. Because the id is not in the signature, the caller cannot supply a
// stale one; this test pins the resolution that replaces it.
//
// It fails against a writer that ignores ServiceInstanceID in the lookup (the
// two instances below share one remote_id and would collide), and against one
// that resolves the link without checking the work is live.
func TestPutPosterAssetResolvesTheWorkInsideItsOwnTransaction(t *testing.T) {
	s := newTestStore(t)
	seedPosterFixture(t, s)

	// A SECOND instance whose own 'b1' names a DIFFERENT work. remote_id is
	// unique only per (instance, kind), so an unscoped lookup picks whichever
	// row it meets first.
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, stmt := range []string{
			`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
			   VALUES (8, 'bookorbit', 'library', 'BookOrbit Two', 'http://books2.example', X'00')`,
			`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
			   VALUES (8, 2, 'b1', 'book', '2026-08-16 09:00:00')`,
		} {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("%s: %w", stmt, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	for _, tc := range []struct {
		instance int64
		wantWork int64
	}{
		{7, 1},
		{8, 2},
	} {
		p := validPoster()
		p.ServiceInstanceID = tc.instance
		p.SourceURL = fmt.Sprintf("http://books%d.example/api/v1/books/11/cover", tc.instance)
		p.CacheKey = fmt.Sprintf("00000000000000%02d", tc.instance)

		id, err := s.PutPosterAsset(t.Context(), p)
		if err != nil {
			t.Fatalf("instance %d: PutPosterAsset: %v", tc.instance, err)
		}
		var poster sql.NullInt64
		if err := s.DB().Read().QueryRowContext(t.Context(),
			`SELECT poster_asset_id FROM work WHERE id = ?`, tc.wantWork).Scan(&poster); err != nil {
			t.Fatalf("reading work %d: %v", tc.wantWork, err)
		}
		if !poster.Valid || poster.Int64 != id {
			t.Errorf("remote_id 'b1' from instance %d landed on the wrong work: work %d has "+
				"poster %v, want %d. The link lookup is not scoped to the instance, so one "+
				"service's cover can overwrite another's.", tc.instance, tc.wantWork, poster, id)
		}
	}

	// A soft-deleted LINK must not resolve either, even though the work is live.
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE service_item_link SET deleted_at = '2026-08-17 00:00:00'
			  WHERE service_instance_id = 8 AND remote_id = 'b1'`)
		return err
	}); err != nil {
		t.Fatalf("tombstoning the link: %v", err)
	}
	p := validPoster()
	p.ServiceInstanceID = 8
	p.SourceURL = "http://books8.example/api/v1/books/99/cover"
	if _, err := s.PutPosterAsset(t.Context(), p); !errors.Is(err, ErrNoSuchWork) {
		t.Errorf("a tombstoned link still resolved: %v", err)
	}
}
