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
	allowedRow := append(append([]string{}, wantRow...), "orphaned_at")
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
