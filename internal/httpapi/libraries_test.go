package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// seedLibrariesScreenCorpus writes §17.8's shape through raw SQL, because the
// endpoint's job is to render whatever the binding put there and the binding is
// a different commit's subject.
//
// A REAL MIGRATED DATABASE WITH ROWS IN IT: the ordering, the Unfiled exclusion
// and the source grouping are all invisible at zero rows.
func seedLibrariesScreenCorpus(t *testing.T, s *Server) {
	t.Helper()
	stmts := []string{
		// TWO instances, so the source list has something to group, and a real
		// base_url plus an api_key_enc — the two service_instance columns this
		// response must never carry.
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (1, 'kavita', 'library', 'Kavita Manga',
		           'http://kavita.internal.example:5000', X'DEADBEEF')`,
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (2, 'kavita', 'library', 'Kavita Books',
		           'http://kavita2.internal.example:5000', X'CAFEBABE')`,

		`INSERT INTO library (id, user_id, name, slug, kind, sort_order, enabled, include_in_search)
		   VALUES (1, 0, 'Manga', 'manga', 'comic', 10, 1, 1)`,
		`INSERT INTO library (id, user_id, name, slug, kind, formats, sort_order,
		                      enabled, include_in_search)
		   VALUES (2, 0, 'Ebooks', 'ebooks', 'book', '["ebook"]', 5, 1, 0)`,
		// §6.5 rule 5's retained orphan: no sources, and an orphaned_at, which
		// nothing in the tree writes but the state is §17.8's.
		`INSERT INTO library (id, user_id, name, slug, kind, sort_order, enabled,
		                      include_in_search, orphaned_at)
		   VALUES (3, 0, 'Loose Ends', 'loose-ends', 'book', 20, 1, 1, '2026-08-17 08:00:00')`,

		`INSERT INTO library_source
		   (id, library_id, service_instance_id, container_kind, container_ref,
		    container_identity, is_metadata_authority)
		 VALUES (1, 1, 1, 'remote_library', '11', 'Manga', 1)`,
		`INSERT INTO library_source
		   (id, library_id, service_instance_id, container_kind, container_ref,
		    container_identity, is_metadata_authority, missing_since)
		 VALUES (2, 2, 1, 'remote_library', '12', 'Books', 1, '2026-08-17 09:30:00')`,
		`INSERT INTO library_source
		   (id, library_id, service_instance_id, container_kind, container_ref,
		    container_identity, is_metadata_authority)
		 VALUES (3, 2, 2, 'remote_library', '21', 'More Books', 0)`,
	}
	for i, lib := range []int{1, 1, 2, 2, 2} {
		id := i + 1
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO work
			  (id, kind, title, sort_title, normalized_title, added_at)
			VALUES (%d, 'book', 'w%02d', 'w%02d', 'w%02d', '2026-08-10 10:00:00')`,
			id, id, id, id))
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO service_item_link (service_instance_id, work_id, remote_id, remote_kind, synced_at)
			 VALUES (1, %d, 'r%d', 'series', '2026-08-16 12:00:00')`, id, id))
		stmts = append(stmts, fmt.Sprintf(
			`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (%d, 'w%02d', %d)`,
			lib, id, id))
	}
	// A member row in the RESERVED library, so an Unfiled leak would show as a
	// count and not only as a name.
	stmts = append(stmts,
		`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (0, 'w01', 1)`)

	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// callListLibraries runs the handler with a session already resolved, which is
// what the authenticated middleware does before it.
func callListLibraries(t *testing.T, s *Server, owner bool) (int, string) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/libraries", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 0, Username: "joe", IsOwner: owner},
	}))
	w := httptest.NewRecorder()
	if err := s.handleListLibraries(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

func TestListLibrariesEndpointRendersTheRowView(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	code, body := callListLibraries(t, s, true)
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/libraries = %d: %s", code, body)
	}
	var got librariesResponse
	mustJSON(t, body, &got)

	// sort_order, not id order.
	var names []string
	for _, l := range got.Items {
		names = append(names, l.Name)
	}
	want := []string{"Ebooks", "Manga", "Loose Ends"}
	if strings.Join(names, " | ") != strings.Join(want, " | ") {
		t.Fatalf("libraries are not in sort_order:\n  got:  %v\n  want: %v", names, want)
	}

	ebooks := got.Items[0]
	if ebooks.Slug != "ebooks" || ebooks.Kind != "book" {
		t.Errorf("Ebooks' identity is wrong: %+v", ebooks)
	}
	if string(ebooks.Formats) != `["ebook"]` {
		t.Errorf("formats = %q, want [\"ebook\"] forwarded verbatim", ebooks.Formats)
	}
	if ebooks.IncludeInSearch || !ebooks.Enabled {
		t.Errorf("Ebooks' two visibility flags are not read independently: %+v", ebooks)
	}
	if ebooks.ItemCount != 3 {
		t.Errorf("Ebooks' item count = %d, want 3", ebooks.ItemCount)
	}
	if len(ebooks.Sources) != 2 {
		t.Fatalf("Ebooks has %d sources, want 2", len(ebooks.Sources))
	}
	if ebooks.Sources[0].ServiceName != "Kavita Manga" ||
		ebooks.Sources[0].ContainerRef != "12" ||
		ebooks.Sources[0].ContainerName == nil || *ebooks.Sources[0].ContainerName != "Books" {
		t.Errorf("the source's binding did not travel: %+v", ebooks.Sources[0])
	}
	if ebooks.Sources[0].MissingSince == nil {
		t.Errorf("the missing source did not report its date: %+v", ebooks.Sources[0])
	}
	if ebooks.Sources[1].MissingSince != nil {
		t.Errorf("a healthy source reported missing_since: %+v", ebooks.Sources[1])
	}

	// A library with no formats column omits the key rather than sending null.
	manga := got.Items[1]
	if manga.Formats != nil {
		t.Errorf("a NULL formats column reached the wire as %q", manga.Formats)
	}

	// The orphan: sources is `[]`, never absent, and orphaned_at travels.
	loose := got.Items[2]
	if loose.Sources == nil || len(loose.Sources) != 0 {
		t.Errorf("the orphaned library's sources are not an empty list: %+v", loose.Sources)
	}
	if loose.OrphanedAt == nil {
		t.Errorf("the orphaned library did not report orphaned_at: %+v", loose)
	}
}

// THE RESERVED ROW, asserted at the HTTP boundary because the boundary is where
// the owner's browser reads it. Migration 0005: "never listed on the Libraries
// screen, never offered in the scope chip, never proposed".
func TestListLibrariesEndpointNeverShipsUnfiled(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	// The row is really there, so the exclusion has something to exclude.
	var name string
	if err := s.store.DB().Read().QueryRowContext(t.Context(),
		`SELECT name FROM library WHERE id = 0`).Scan(&name); err != nil {
		t.Fatalf("the reserved row is not in the database, so this test asserts nothing: %v", err)
	}

	_, body := callListLibraries(t, s, true)
	if strings.Contains(body, "Unfiled") || strings.Contains(body, `"slug":"unfiled"`) {
		t.Errorf("the reserved Unfiled row reached the browser: %s", body)
	}
	var got librariesResponse
	mustJSON(t, body, &got)
	for _, l := range got.Items {
		if l.ID == 0 {
			t.Errorf("library 0 reached the browser: %+v", l)
		}
	}
}

// THE ALLOWLIST, pinned as a key set on both structs. The response structs ARE
// the allowlist and this is what makes growing either one deliberate: a column
// added to store.Library and copied across by a later change fails here first.
func TestLibrariesResponseKeysAreTheAllowlist(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	_, body := callListLibraries(t, s, true)
	var envelope map[string]json.RawMessage
	mustJSON(t, body, &envelope)
	if got := keysOf(envelope); strings.Join(got, ",") != "items" {
		t.Errorf("envelope keys = %v, want [items]", got)
	}

	var items []map[string]json.RawMessage
	mustJSON(t, string(envelope["items"]), &items)
	if len(items) == 0 {
		t.Fatal("no items to inspect")
	}

	// Ebooks carries every optional key — formats and a source with a
	// missing_since — so every other row is a subset of it.
	wantRow := []string{
		"enabled", "formats", "id", "include_in_search", "item_count", "kind",
		"name", "slug", "sort_order", "sources",
	}
	if got := keysOf(items[0]); strings.Join(got, ",") != strings.Join(wantRow, ",") {
		t.Errorf("row keys = %v\n         want %v", got, wantRow)
	}
	allowedRow := append(append([]string{}, wantRow...), "orphaned_at", "completeness", "skipped")
	for i, item := range items {
		for key := range item {
			if !contains(allowedRow, key) {
				t.Errorf("row %d carries %q, which is not on the allowlist", i, key)
			}
		}
	}

	var sources []map[string]json.RawMessage
	mustJSON(t, string(items[0]["sources"]), &sources)
	if len(sources) == 0 {
		t.Fatal("no sources to inspect")
	}
	wantSource := []string{
		"container_kind", "container_name", "container_ref", "id",
		"is_metadata_authority", "missing_since", "service_instance_id",
		"service_kind", "service_name",
	}
	if got := keysOf(sources[0]); strings.Join(got, ",") != strings.Join(wantSource, ",") {
		t.Errorf("source keys = %v\n            want %v", got, wantSource)
	}
	for i, src := range sources {
		for key := range src {
			if !contains(wantSource, key) {
				t.Errorf("source %d carries %q, which is not on the allowlist", i, key)
			}
		}
	}

	// The two fields ADR-0048 and §17.8 rule OFF this wire, named individually
	// so removing one from the argument is not silently symmetrical.
	for _, forbidden := range []string{"managed_by", "proposed", "accepted_at"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%q reached the browser; ADR-0048 decides a proposal is not a row, "+
				"so there is no state for it to carry: %s", forbidden, body)
		}
	}
}

// Nothing credential-shaped and nothing service-side leaves, asserted against
// the RESPONSE BYTES rather than against the struct, so a field added by any
// route — including one added by embedding — is caught. This is the endpoint
// where it matters most: it is the only read in the product that joins a
// user-facing list to `service_instance`, the row holding a full-admin *Arr key.
func TestListLibrariesShipsNoCredentialOrAddress(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	_, body := callListLibraries(t, s, true)
	for _, forbidden := range []string{
		"api_key", "apikey", "api_key_enc", "deadbeef", "DEADBEEF", "cafebabe", "CAFEBABE",
		"base_url", "kavita.internal.example", "kavita2.internal.example", "5000",
		"sink_", "quality_profile", "root_folder",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%q reached the browser: %s", forbidden, body)
		}
	}
}

// THE SCOPE, at the HTTP boundary. storeScope fails closed for a non-owner, and
// this endpoint publishes service instance names, so "fails closed" has to mean
// no rows rather than all rows.
func TestListLibrariesEndpointScopeFailsClosed(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	code, body := callListLibraries(t, s, false)
	if code != http.StatusOK {
		t.Fatalf("non-owner GET = %d: %s", code, body)
	}
	var got librariesResponse
	mustJSON(t, body, &got)
	if len(got.Items) != 0 {
		t.Fatalf("a non-owner saw %d libraries: %s", len(got.Items), body)
	}
	// `items` is present and empty, never absent: an absent key is
	// indistinguishable from a failure.
	if !strings.Contains(body, `"items":[]`) {
		t.Errorf("the empty response is not an empty list: %s", body)
	}

	// And the owner sees them, so the assertion above is about the scope and
	// not about an empty database.
	if _, ownerBody := callListLibraries(t, s, true); !strings.Contains(ownerBody, "Manga") {
		t.Fatalf("the owner sees nothing either, so this test proves nothing: %s", ownerBody)
	}
}

// The route is registered, authenticated, and reachable at the path clients will
// use — asserted through the real mux rather than by calling the handler, which
// is what a route-table typo would slip past.
func TestListLibrariesRouteIsRegisteredAndAuthenticated(t *testing.T) {
	s := newTestServer(t, nil)
	s.MarkListening()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	if code, _ := get(t, srv.URL+"/api/v1/libraries"); code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/libraries unauthenticated = %d, want 401", code)
	}
}

// A `formats` value that is not a JSON array is DROPPED rather than forwarded,
// so a renderer whose type says "list of strings" never receives something else.
func TestListLibrariesDropsAMalformedFormats(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE library SET formats = '{"not":"an array"}' WHERE id = 2`)
		return err
	}); err != nil {
		t.Fatalf("write bad formats: %v", err)
	}

	_, body := callListLibraries(t, s, true)
	if strings.Contains(body, "not an array") || strings.Contains(body, `"not"`) {
		t.Errorf("a malformed formats value was forwarded: %s", body)
	}
	var got librariesResponse
	mustJSON(t, body, &got)
	if got.Items[0].Formats != nil {
		t.Errorf("formats = %q, want the key omitted", got.Items[0].Formats)
	}
	// And the rest of the row survived: dropping one decoration must not trade
	// the whole response for it.
	if got.Items[0].Name != "Ebooks" || got.Items[0].ItemCount != 3 {
		t.Errorf("the row was damaged by the drop: %+v", got.Items[0])
	}
}

// ─── the content-filter shortfall ────────────────────────────────────────────

// seedCompleteness writes one content_completeness sync_report row against the
// library_source whose (service_instance_id, container_ref) pair matches.
//
// It writes the ROW rather than calling the adapter, because what is under test
// here is the read and the wire — the measurement is internal/libsync's and is
// tested there.
func seedCompleteness(t *testing.T, s *Server, instanceID int, ref, detail string) {
	t.Helper()
	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO sync_report (service_instance_id, kind, remote_kind, remote_id, detail)
			VALUES (?, ?, 'library', ?, ?)`,
			instanceID, store.SyncReportContentCompleteness, ref, detail)
		return err
	}); err != nil {
		t.Fatalf("seed completeness for %d/%s: %v", instanceID, ref, err)
	}
}

// TestAContentFilterShortfallReachesTheLibrariesWire is the end of the chain
// this feature exists for: a BookOrbit content filter shorts a library's book
// count, the import records the subtraction, and the Libraries screen can say
// so.
//
// ⚠️ IT ALSO PINS THE STATE THAT KEEPS THE OTHER TWO HONEST. `unverified` must
// carry NO counts — not zeroes, no key at all — because the check rests on
// BookOrbit's `GET /libraries/:id/stats` being unguarded, and on the day that
// changes every verdict becomes this one. A response that served
// `total_items: 0` there would report an empty library, and a response that
// omitted the object entirely would report no problem at all.
func TestAContentFilterShortfallReachesTheLibrariesWire(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	// Library 1 (Manga) binds instance 1 / container '11': a real shortfall.
	seedCompleteness(t, s, 1, "11", `{"state":"shortfall","container":"Manga",
		"total_items":412,"visible_items":389,"hidden_items":23,
		"covers":"c","does_not_cover":"d"}`)
	// Library 3 (Loose Ends) has no sources at all, so nothing can be measured
	// about it and nothing may be claimed.

	_, body := callListLibraries(t, s, true)
	var got librariesResponse
	mustJSON(t, body, &got)

	byName := map[string]libraryResponse{}
	for _, l := range got.Items {
		byName[l.Name] = l
	}

	manga := byName["Manga"].Completeness
	if manga == nil {
		t.Fatalf("Manga carries no completeness verdict; body = %s", body)
	}
	if manga.State != "shortfall" || manga.Total != 412 || manga.Visible != 389 || manga.Hidden != 23 {
		t.Errorf("Manga's verdict = %+v, want shortfall 412/389/23", manga)
	}
	if manga.CheckedAt == nil {
		t.Error("a stored measurement reached the wire with no checked_at; " +
			"a shortfall with no age reads as a live fact rather than as a measurement")
	}

	// ⚠️ THE LIBRARY THAT WAS NEVER MEASURED CARRIES NOTHING. Ebooks' two
	// sources have no completeness row, and the key must be ABSENT rather than
	// an object saying complete — an unmeasured library that rendered as fine
	// would be the exact defect this feature closes.
	if c := byName["Ebooks"].Completeness; c != nil {
		t.Errorf("Ebooks was never measured and still carries %+v", c)
	}
	if c := byName["Loose Ends"].Completeness; c != nil {
		t.Errorf("a library with no sources carries %+v", c)
	}
}

func TestAnUnverifiedCompletenessNeverPublishesACount(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	// The guard-later scenario: BookOrbit put a permission guard on the stats
	// route, so the probe refused and nothing could be compared.
	seedCompleteness(t, s, 1, "11", `{"state":"unverified","container":"Manga",
		"total_items":-1,"visible_items":389,"hidden_items":0,
		"reason":"BookOrbit refused this account the library statistics UsArr compares against"}`)

	_, body := callListLibraries(t, s, true)
	var raw struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	mustJSON(t, body, &raw)

	var found bool
	for _, item := range raw.Items {
		if string(item["name"]) != `"Manga"` {
			continue
		}
		found = true
		blob, ok := item["completeness"]
		if !ok {
			t.Fatal("an unverified verdict was dropped from the wire; absence reads as " +
				"'no problem', which is exactly what unverified must not mean")
		}
		var c map[string]json.RawMessage
		mustJSON(t, string(blob), &c)
		for _, key := range []string{"total_items", "visible_items", "hidden_items"} {
			if _, present := c[key]; present {
				t.Errorf("unverified carries %q = %s; a count nobody measured must not travel",
					key, c[key])
			}
		}
		if _, present := c["reason"]; !present {
			t.Error("unverified carries no reason, so the screen cannot say why")
		}
		if string(c["state"]) != `"unverified"` {
			t.Errorf("state = %s, want \"unverified\"", c["state"])
		}
	}
	if !found {
		t.Fatalf("Manga is not in the response: %s", body)
	}
}

// TestAnUnverifiedContainerOutranksAShortfallInTheSameLibrary pins the fold.
//
// Ebooks binds TWO containers. One reports a clean shortfall and the other could
// not be measured at all, and the library-level answer is `unverified` — because
// a library-level "holds 412, sees 389" while a second container was never
// looked at is a precise-sounding claim that is not true.
func TestAnUnverifiedContainerOutranksAShortfallInTheSameLibrary(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	seedCompleteness(t, s, 1, "12", `{"state":"shortfall","total_items":100,
		"visible_items":90,"hidden_items":10}`)
	seedCompleteness(t, s, 2, "21", `{"state":"unverified","total_items":-1,
		"visible_items":40,"reason":"the statistics could not be read"}`)

	_, body := callListLibraries(t, s, true)
	var got librariesResponse
	mustJSON(t, body, &got)
	for _, l := range got.Items {
		if l.Name != "Ebooks" {
			continue
		}
		if l.Completeness == nil || l.Completeness.State != "unverified" {
			t.Fatalf("Ebooks' folded verdict = %+v, want unverified", l.Completeness)
		}
		if l.Completeness.Total != 0 || l.Completeness.Hidden != 0 {
			t.Errorf("a partial sum was published under an unverified label: %+v", l.Completeness)
		}
	}
}

// TestAnUnreadableCompletenessRowIsDroppedRatherThanGuessed covers the two ways
// a stored verdict can be unusable. Both must render as "nothing was measured";
// neither may be coerced to the nearest member.
func TestAnUnreadableCompletenessRowIsDroppedRatherThanGuessed(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	seedCompleteness(t, s, 1, "11", `not json at all`)
	seedCompleteness(t, s, 1, "12", `{"state":"probably fine","total_items":9}`)

	_, body := callListLibraries(t, s, true)
	if strings.Contains(body, "probably fine") {
		t.Errorf("an unknown state reached the browser: %s", body)
	}
	var got librariesResponse
	mustJSON(t, body, &got)
	for _, l := range got.Items {
		if l.Completeness != nil {
			t.Errorf("%s carries %+v from an unreadable row", l.Name, l.Completeness)
		}
		// And the row itself survived: a decoration that will not parse must not
		// cost the library.
		if l.Name == "" {
			t.Error("a row was damaged by the drop")
		}
	}
}

// ─── what the import read and did not map ────────────────────────────────────

// seedSkip writes one items_skipped sync_report row against the library_source
// whose (service_instance_id, container_ref) pair matches, on seedCompleteness's
// reasoning: what is under test is the read and the wire, and the tally itself
// is internal/libsync's.
func seedSkip(t *testing.T, s *Server, instanceID int, ref, detail string) {
	t.Helper()
	if err := s.store.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO sync_report (service_instance_id, kind, remote_kind, remote_id, detail)
			VALUES (?, ?, 'library', ?, ?)`,
			instanceID, store.SyncReportItemsSkipped, ref, detail)
		return err
	}); err != nil {
		t.Fatalf("seed skip for %d/%s: %v", instanceID, ref, err)
	}
}

// THE END OF THE CHAIN THIS FEATURE EXISTS FOR: an adapter reads a comic, has no
// unit of work for it, counts it, and the Libraries screen can say so without
// anybody opening the database.
//
// ⚠️ AND IT PINS THE THREE READINGS APART ON THE WIRE, WHICH IS THE PART THAT IS
// EASY TO LOSE. "The walk left nothing out" is `state: "none"` and "nothing ever
// walked it" is no `skipped` key at all. Collapse them and an absent record
// starts reading as an all-clear, which is the defect ADR-0061 exists to prevent
// one axis over.
//
// ⚠️ THE WIRE VALUES ARE UNCHANGED BY ADR-0063 AND THE EVIDENCE IS NOT, which is
// why the seed below moved and this assertion did not. `none` used to be derived
// from a completeness row standing in for "somebody walked this"; it now rests
// on a zero-count skip row of its own.
func TestWhatTheImportLeftOutReachesTheLibrariesWire(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)

	// Library 1 (Manga) binds instance 1 / container '11': items were left out.
	seedSkip(t, s, 1, "11", `{"name":"Manga","skipped_comics":2,"skipped_unknown":1,
		"reason":"UsArr maps prose books only","effect":"e","covers":"c","does_not_cover":"d"}`)
	seedCompleteness(t, s, 1, "11", `{"state":"complete","total_items":9,"visible_items":9}`)
	// Library 2 (Ebooks) binds both containers, both WALKED, neither skipped —
	// so each carries a ZERO-COUNT skip row of its own (ADR-0063). The
	// completeness verdicts beside them are the sibling axis and are no longer
	// what makes `none` legible.
	seedSkip(t, s, 1, "12", `{"name":"Ebooks","skipped_comics":0,"skipped_unknown":0}`)
	seedSkip(t, s, 2, "21", `{"name":"More Ebooks","skipped_comics":0,"skipped_unknown":0}`)
	seedCompleteness(t, s, 1, "12", `{"state":"complete","total_items":3,"visible_items":3}`)
	seedCompleteness(t, s, 2, "21", `{"state":"complete","total_items":1,"visible_items":1}`)
	// Library 3 (Loose Ends) has no sources at all, so nothing walked it.

	_, body := callListLibraries(t, s, true)
	var got librariesResponse
	mustJSON(t, body, &got)

	byName := map[string]libraryResponse{}
	for _, l := range got.Items {
		byName[l.Name] = l
	}

	manga := byName["Manga"].Skipped
	if manga == nil || manga.State != "left_out" {
		t.Fatalf("Manga's skipped = %+v, want a left_out verdict: %s", manga, body)
	}
	if manga.Items != 3 {
		t.Errorf("Manga left out %d items, want 3 (2 comics + 1 unknown)", manga.Items)
	}
	if manga.Reason != "UsArr maps prose books only" {
		t.Errorf("Manga's reason = %q, want UsArr's own short sentence", manga.Reason)
	}
	if manga.RecordedAt == nil {
		t.Error("no recorded_at, so a client cannot render the count as a measurement with an age")
	}

	// ⚠️ THE ADAPTER'S OWN VOCABULARY MUST NOT CROSS. `skipped_comics` and
	// `skipped_unknown` are BookOrbit's words for BookOrbit's two reasons; a
	// second adapter declines items for neither, and an API field named `comics`
	// would then have to be lied to. The split stays in sync_report.detail.
	if strings.Contains(body, "skipped_comics") || strings.Contains(body, "comics") {
		t.Errorf("the adapter's per-reason vocabulary reached the wire: %s", body)
	}
	// And nothing operator-facing crossed either: `effect`, `covers` and
	// `does_not_cover` live in the row for whoever reads the database.
	if strings.Contains(body, "does_not_cover") || strings.Contains(body, `"effect"`) {
		t.Errorf("an operator-only key reached the wire: %s", body)
	}

	// ── the two silences, which are NOT one value ──────────────────────────
	ebooks := byName["Ebooks"].Skipped
	if ebooks == nil || ebooks.State != "none" {
		t.Fatalf("Ebooks' skipped = %+v, want `none` — both its containers were walked "+
			"and each recorded a zero, which is a MEASURED negative: %s", ebooks, body)
	}
	if ebooks.Items != 0 || ebooks.Reason != "" {
		t.Errorf("the `none` verdict published a count or a reason: %+v", ebooks)
	}
	if loose := byName["Loose Ends"].Skipped; loose != nil {
		t.Errorf("Loose Ends carries %+v — nothing has ever walked it, and a verdict "+
			"there reports silence as a measurement", loose)
	}

	// ⚠️ ON THE WIRE, NOT MERELY IN THE STRUCT. `items` and `reason` must be
	// ABSENT under `none` rather than served as a zero and an empty string: a
	// count of 0 under that label is a claim the label does not make. The scan
	// below is over the raw body because that is what a client parses.
	var raw struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	mustJSON(t, body, &raw)
	for i, l := range raw.Items {
		blob, ok := l["skipped"]
		if !ok {
			continue
		}
		var fields map[string]json.RawMessage
		mustJSON(t, string(blob), &fields)
		if string(fields["state"]) != `"none"` {
			continue
		}
		for _, key := range []string{"items", "reason"} {
			if _, present := fields[key]; present {
				t.Errorf("row %d serves %q under state `none`: %s", i, key, blob)
			}
		}
	}
}

// A skip verdict whose state this build does not know is DROPPED, not rendered
// and not guessed at.
//
// The store reads its state out of a JSON blob no constraint governs — sync_report
// carries no CHECK over `kind` and `detail` is untyped — so the vocabulary can
// grow underneath a running binary. The only honest rendering of a verdict this
// build cannot read is NO verdict, which is what an unobserved library gets, and
// it is the one reading that cannot overstate.
func TestAnUnknownSkipStateIsNotPublished(t *testing.T) {
	s := newTestServer(t, nil)
	seedLibrariesScreenCorpus(t, s)
	seedCompleteness(t, s, 1, "11", `{"state":"complete","total_items":9,"visible_items":9}`)

	// Straight past the store's own decode, to prove the handler refuses too:
	// the two sides deploy independently.
	rows := []store.Library{{
		ID:    1,
		Name:  "Manga",
		Skips: &store.LibrarySkips{State: store.SkipState("mostly"), Items: 40},
	}}
	if got := librarySkipsFor(rows[0]); got != nil {
		t.Errorf("librarySkipsFor published %+v for a state no member matches", got)
	}
}
