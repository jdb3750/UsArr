package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jdb3750/UsArr/internal/store"
)

// ─────────────────────────────────────────────────────────────────────────────
// The corpus for §17.8's Accept step at the HTTP boundary.
//
// A REAL MIGRATED DATABASE WITH ROWS IN IT, seeded through raw SQL for
// seedLibrariesScreenCorpus's reason: what these handlers owe is the rendering
// of whatever the bind transaction put there, and the bind transaction is a
// different package's subject.
//
//	instance 1  Kavita One   — base_url and api_key_enc set, so a leak is visible
//	  c-a  Alpha (comic)     works 1, 2       — already a `book` library (300)
//	  c-b  Beta  (book)      work 3           — a `book` library named Beta exists
//	  c-x  Podcasts          declined, with a reason, and stale
//	instance 2  Kavita Two
//	  c-z  Zed   (comic)     work 8           — already a `comic` library (100)
//
// So the response has: a proposal bound at ANOTHER kind (c-a, ADR-0066
// decision 5), a proposal that would JOIN rather than create (c-b), a DECLINED
// proposal (c-x) that is also the one the last sync did not report, and one
// container that is not a proposal at all (c-z, bound at its own kind).
// ─────────────────────────────────────────────────────────────────────────────

// observation renders one `container_observed` sync_report row.
//
// ⚠️ THE DETAIL BLOB IS HAND-WRITTEN AND THE DRIFT IS CAUGHT BY THE ASSERTIONS,
// not by this helper. store.recordContainerObservation is unexported, so this
// test cannot call the shipped writer the way internal/store's own tests do; a
// field renamed on the store's side would decode into a zero value rather than
// erroring, so TestLibraryProposalsEndpointRendersTheAcceptStep asserts the NAME
// and the KIND of a proposal by value — an empty one is a red test rather than a
// plausible row.
func observation(instanceID int64, ref, name, kind, declineReason string, provisional bool) string {
	detail, err := json.Marshal(map[string]any{
		"name": name, "kind": kind,
		"kind_provisional": provisional, "decline_reason": declineReason,
	})
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`INSERT INTO sync_report
		  (service_instance_id, kind, remote_kind, remote_id, detail)
		VALUES (%d, '%s', 'library', '%s', '%s')`,
		instanceID, store.SyncReportContainerObserved, ref, string(detail))
}

func seedProposalsScreenCorpus(t *testing.T, s *Server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc, last_full_sync_at)
		   VALUES (1, 'kavita', 'library', 'Kavita One',
		           'http://one.internal.example:5000', X'DEADBEEF', '2026-02-01 00:00:00')`,
		`INSERT INTO service_instance (id, kind, role, name, base_url, api_key_enc)
		   VALUES (2, 'kavita', 'library', 'Kavita Two',
		           'http://two.internal.example:5000', X'CAFEBABE')`,
	}
	works := []struct {
		id        int64
		kind      string
		instance  int64
		container string
	}{
		{1, "comic", 1, "c-a"},
		{2, "comic", 1, "c-a"},
		{3, "book", 1, "c-b"},
		{8, "comic", 2, "c-z"},
	}
	for _, w := range works {
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO work
			  (id, kind, title, sort_title, normalized_title, added_at)
			VALUES (%d, '%s', 'w%02d', 'w%02d', 'w%02d', '2026-08-10 10:00:00')`,
			w.id, w.kind, w.id, w.id, w.id))
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO service_item_link
			  (service_instance_id, work_id, remote_id, remote_kind, remote_library_id, synced_at)
			VALUES (%d, %d, 'r%02d', 'series', '%s', '2026-08-16 12:00:00')`,
			w.instance, w.id, w.id, w.container))
	}
	stmts = append(stmts,
		// Bound at its OWN kind: c-z is not a proposal.
		`INSERT INTO library (id, user_id, name, slug, kind, managed_by)
		   VALUES (100, 0, 'Existing Comics', 'existing-comics', 'comic', 'auto')`,
		`INSERT INTO library_source
		   (library_id, service_instance_id, container_kind, container_ref, container_identity)
		 VALUES (100, 2, 'remote_library', 'c-z', 'Two''s comics')`,
		`INSERT INTO library_member (library_id, sort_title, work_id) VALUES (100, 'w08', 8)`,
		// Bound at ANOTHER kind: c-a is still a proposal, and it says what it
		// would sit beside.
		`INSERT INTO library (id, user_id, name, slug, kind, managed_by)
		   VALUES (300, 0, 'Alpha prose', 'alpha-prose', 'book', 'auto')`,
		`INSERT INTO library_source
		   (library_id, service_instance_id, container_kind, container_ref, container_identity)
		 VALUES (300, 1, 'remote_library', 'c-a', 'Alpha')`,
		// The join prediction: c-b's suggested name is `Beta`, and a book
		// library already holds it.
		`INSERT INTO library (id, user_id, name, slug, kind, managed_by)
		   VALUES (200, 0, 'Beta', 'beta', 'book', 'auto')`,

		observation(1, "c-a", "Alpha", "comic", "", false),
		observation(1, "c-b", "Beta", "book", "", true),
		observation(1, "c-x", "Podcasts", "", "no work.kind for a podcast", false),
		observation(2, "c-z", "Zed", "comic", "", false),
		// c-x was last reported BEFORE instance 1's last completed full sync, so
		// it is the one proposal flagged as no longer reported. The others keep
		// SQLite's `datetime('now')` default, which is after the stamp.
		fmt.Sprintf(`UPDATE sync_report SET created_at = '2026-01-01 00:00:00'
		    WHERE remote_id = 'c-x' AND kind = '%s'`, store.SyncReportContainerObserved),
	)

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

func sessionRequest(t *testing.T, method, path, body string, owner bool) *http.Request {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequestWithContext(t.Context(), method, path, nil)
	} else {
		r = httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	}
	return r.WithContext(context.WithValue(r.Context(), ctxKeySession, authSession{
		User: store.User{ID: 0, Username: "joe", IsOwner: owner},
	}))
}

// callProposals runs the read with a session already resolved, which is what the
// authenticated middleware does before it.
func callProposals(t *testing.T, s *Server, owner bool) (int, string) {
	t.Helper()
	r := sessionRequest(t, http.MethodGet, "/api/v1/libraries/proposals", "", owner)
	w := httptest.NewRecorder()
	if err := s.handleListLibraryProposals(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

func callAccept(t *testing.T, s *Server, owner bool, body string) (int, string) {
	t.Helper()
	r := sessionRequest(t, http.MethodPost, "/api/v1/libraries/accept", body, owner)
	w := httptest.NewRecorder()
	if err := s.handleAcceptLibraries(w, r); err != nil {
		s.writeError(w, r, err)
	}
	return w.Code, w.Body.String()
}

func proposalWithRef(t *testing.T, body, ref string) containerProposalResponse {
	t.Helper()
	var got proposalsResponse
	mustJSON(t, body, &got)
	for _, p := range got.Items {
		if p.ContainerRef == ref {
			return p
		}
	}
	t.Fatalf("no proposal for container %q in %s", ref, body)
	return containerProposalResponse{}
}

func proposalRefsOnTheWire(t *testing.T, body string) []string {
	t.Helper()
	var got proposalsResponse
	mustJSON(t, body, &got)
	out := make([]string, 0, len(got.Items))
	for _, p := range got.Items {
		out = append(out, fmt.Sprintf("%d/%s", p.ServiceInstanceID, p.ContainerRef))
	}
	return out
}

// THE HEADLINE: §17.8's Accept step, whole, from the local file.
//
// ⚠️ ONE `if` PER FIELD, NEVER A `switch` — internal/store's proposals_test.go
// states the reason: a switch stops at the first true case, so a change that
// blanked three fields would report one of them per run.
func TestLibraryProposalsEndpointRendersTheAcceptStep(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	code, body := callProposals(t, s, true)
	if code != http.StatusOK {
		t.Fatalf("GET = %d: %s", code, body)
	}
	want := []string{"1/c-a", "1/c-b", "1/c-x"}
	if got := proposalRefsOnTheWire(t, body); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("proposed %v, want %v: %s", got, want, body)
	}

	// c-a: bound at ANOTHER kind, so it is still a proposal and it says what it
	// would sit beside (ADR-0066 decision 5).
	a := proposalWithRef(t, body, "c-a")
	if a.ContainerName != "Alpha" {
		t.Errorf("container_name = %q, want the name upstream reported", a.ContainerName)
	}
	if a.Kind != "comic" {
		t.Errorf("kind = %q, want comic", a.Kind)
	}
	if a.ServiceName != "Kavita One" {
		t.Errorf("service_name = %q, want Kavita One", a.ServiceName)
	}
	if a.ServiceKind != "kavita" {
		t.Errorf("service_kind = %q, want kavita", a.ServiceKind)
	}
	if a.ItemCount != 2 {
		t.Errorf("item_count = %d, want 2", a.ItemCount)
	}
	if a.SuggestedName != "Alpha" {
		t.Errorf("suggested_name = %q, want Alpha", a.SuggestedName)
	}
	if a.Declined {
		t.Errorf("c-a reads as declined although the adapter gave it a kind")
	}
	if a.NotSeenByLastSync {
		t.Errorf("c-a is flagged as no longer reported although it was observed after the sync")
	}
	if a.ObservedAt == nil {
		t.Errorf("observed_at is absent, so the screen cannot date the proposal at all")
	}
	if len(a.BoundTo) != 1 || a.BoundTo[0].ID != 300 || a.BoundTo[0].Kind != "book" {
		t.Errorf("bound_to = %+v, want the book library 300", a.BoundTo)
	}
	if a.Joins != nil {
		t.Errorf("joins = %+v; `Alpha` names no comic library, so accepting it creates one", a.Joins)
	}

	// c-b: the suggested name is already held AT THE SAME KIND, so accepting it
	// as suggested joins rather than creates.
	b := proposalWithRef(t, body, "c-b")
	if !b.KindProvisional {
		t.Errorf("kind_provisional = false; the adapter's guess flag did not survive the wire")
	}
	if b.Joins == nil || b.Joins.ID != 200 || b.Joins.Name != "Beta" {
		t.Errorf("joins = %+v, want library 200 Beta", b.Joins)
	}
	if len(b.BoundTo) != 0 {
		t.Errorf("bound_to = %+v, want empty", b.BoundTo)
	}

	// c-x: declined WITH ITS REASON — §17.8's `Decision` column — and stale.
	x := proposalWithRef(t, body, "c-x")
	if !x.Declined {
		t.Errorf("c-x does not read as declined although the adapter gave it no kind")
	}
	if x.Kind != "" {
		t.Errorf("kind = %q on a declined container", x.Kind)
	}
	if x.DeclineReason != "no work.kind for a podcast" {
		t.Errorf("decline_reason = %q, want the adapter's reason", x.DeclineReason)
	}
	if !x.NotSeenByLastSync {
		t.Errorf("c-x is not flagged, although the last completed sync did not report it")
	}
}

// A container already bound AT ITS OWN KIND is not a proposal, and the wire must
// not offer it: accepting it would be a no-op.
func TestLibraryProposalsNeverOfferAContainerBoundAtItsOwnKind(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	_, body := callProposals(t, s, true)
	for _, ref := range proposalRefsOnTheWire(t, body) {
		if ref == "2/c-z" {
			t.Errorf("c-z is proposed although library 100 already stands over it: %s", body)
		}
	}
}

// THE ALLOWLIST, pinned as a key set. The response structs ARE the allowlist and
// this is what makes growing one deliberate: a field added to
// store.ContainerProposal and copied across by a later change fails here first.
func TestProposalsResponseKeysAreTheAllowlist(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	_, body := callProposals(t, s, true)
	var envelope map[string]json.RawMessage
	mustJSON(t, body, &envelope)
	if got := keysOf(envelope); strings.Join(got, ",") != "items" {
		t.Errorf("envelope keys = %v, want [items]", got)
	}

	var items []map[string]json.RawMessage
	mustJSON(t, string(envelope["items"]), &items)
	if len(items) != 3 {
		t.Fatalf("want three proposals to inspect, got %d: %s", len(items), body)
	}

	// c-b carries every optional key except `decline_reason`, which only a
	// declined container has, so the two together cover the struct.
	wantRow := []string{
		"bound_to", "container_name", "container_ref", "declined", "item_count",
		"joins", "kind", "kind_provisional", "not_seen_by_last_sync", "observed_at",
		"service_instance_id", "service_kind", "service_name", "suggested_name",
	}
	allowed := append(append([]string{}, wantRow...), "decline_reason")
	if got := keysOf(items[1]); strings.Join(got, ",") != strings.Join(wantRow, ",") {
		t.Errorf("row keys = %v\n         want %v", got, wantRow)
	}
	for i, item := range items {
		for key := range item {
			if !contains(allowed, key) {
				t.Errorf("proposal %d carries %q, which is not on the allowlist", i, key)
			}
		}
	}

	var bound []map[string]json.RawMessage
	mustJSON(t, string(items[0]["bound_to"]), &bound)
	if len(bound) != 1 {
		t.Fatalf("want one bound library to inspect: %s", string(items[0]["bound_to"]))
	}
	if got := keysOf(bound[0]); strings.Join(got, ",") != "id,kind,name" {
		t.Errorf("bound_to keys = %v, want [id kind name]", got)
	}

	var joins map[string]json.RawMessage
	mustJSON(t, string(items[1]["joins"]), &joins)
	if got := keysOf(joins); strings.Join(got, ",") != "id,name" {
		t.Errorf("joins keys = %v, want [id name]", got)
	}

	// Named individually, because each is a field a later change might think is
	// harmless. `managed_by` is ADR-0048's closed question; `instance_last_full_sync_at`
	// is /services/health's field and the input to a flag this response already
	// decides; `container_kind` has exactly one legal value here.
	for _, forbidden := range []string{
		"managed_by", "instance_last_full_sync_at", "container_kind", "proposal_id",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%q reached the browser: %s", forbidden, body)
		}
	}
}

// Nothing credential-shaped and nothing service-side leaves, asserted against the
// RESPONSE BYTES rather than against the struct, so a field added by any route —
// including one added by embedding — is caught.
//
// THIS IS THE §14 TEST FOR THIS ENDPOINT. A proposal names the service instance
// that reported the container, and `service_instance` is the row holding a
// full-admin *Arr credential and an internal host the user typed.
// internal/store's TestTheProposalStatementReadsNoCredentialColumn is the other
// half: it keeps the columns off the READ, and this keeps them off the WIRE.
func TestLibraryProposalsShipNoCredentialOrAddress(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	_, body := callProposals(t, s, true)
	if !strings.Contains(body, "Kavita One") {
		t.Fatalf("the response names no service at all, so this test proves nothing: %s", body)
	}
	for _, forbidden := range []string{
		"api_key", "apikey", "api_key_enc", "deadbeef", "DEADBEEF", "cafebabe", "CAFEBABE",
		"base_url", "one.internal.example", "two.internal.example", "5000",
		"tls_spki_pin", "kek", "salt", "password", "token",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%q reached the browser: %s", forbidden, body)
		}
	}
}

// THE ACCESS SCOPE, at the HTTP boundary. It is the twin of internal/store's
// TestProposedContainersScopeGuardFires: that one neutralises the predicate in
// the statement, this one asserts that the handler passes a scope derived from
// the SESSION at all. Replacing storeScope(a) with an owner scope turns this red.
func TestLibraryProposalsScopeFailsClosed(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	code, body := callProposals(t, s, false)
	if code != http.StatusOK {
		t.Fatalf("non-owner GET = %d: %s", code, body)
	}
	var got proposalsResponse
	mustJSON(t, body, &got)
	if len(got.Items) != 0 {
		t.Fatalf("a caller who can see no instance was offered %d containers: %s",
			len(got.Items), body)
	}
	// A caller who cannot see the instance is not told it has containers, and is
	// not told the instance exists either.
	for _, forbidden := range []string{"Kavita One", "Kavita Two", "c-a", "c-b", "c-x"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%q reached a caller outside the scope: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"items":[]`) {
		t.Errorf("the empty response is not an empty list: %s", body)
	}
	// And the owner sees them, so the assertion above is about the scope and not
	// about an empty database.
	if _, ownerBody := callProposals(t, s, true); !strings.Contains(ownerBody, "c-a") {
		t.Fatalf("the owner is offered nothing either, so this test proves nothing: %s", ownerBody)
	}
}

// ⚠️ AND NEITHER HANDLER CALLS ANYTHING OUTBOUND, which is the property
// principle 1 rests on and the one a signature cannot state.
//
// The check is structural rather than behavioural, on internal/store's
// TestProposalsReachNoAdapterAndNoSyncPackage's reasoning: a proposal read that
// can block on a Kavita being off is the shape ADR-0048 clause 5 excuses ONCE, at
// connect, and never on a settings screen. It is asserted per FILE rather than
// per package, because this package legitimately reaches Prowlarr from grab.go —
// what must stay true is that these two handlers do not.
func TestLibraryProposalHandlersReachNothingOutbound(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "proposals.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse proposals.go: %v", err)
	}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, forbidden := range []string{
			"libsync", "adapter", "kavita", "prowlarr", "servarr", "releases", "ssrf",
		} {
			if strings.Contains(path, forbidden) {
				t.Errorf("proposals.go imports %q. The Accept screen renders from local SQLite; "+
					"a package that can make a request is one refactor away from doing it on a "+
					"render path.", path)
			}
		}
	}

	// The ports are reached through the Config rather than through an import, so
	// the import list alone would not catch a handler that grew a probe.
	src, err := os.ReadFile("proposals.go")
	if err != nil {
		t.Fatalf("read proposals.go: %v", err)
	}
	for _, port := range []string{"cfg.Releases", "cfg.Tester", "cfg.Probes", "cfg.Imports"} {
		if strings.Contains(string(src), port) {
			t.Errorf("proposals.go reaches %s, which is an upstream call on a render path", port)
		}
	}
}

// ── POST /api/v1/libraries/accept ───────────────────────────────────────────

// acceptBody renders one acceptance, so the tests below differ only in what they
// are testing.
func acceptBody(name, kind string, edited bool, instanceID int64, ref string) string {
	return fmt.Sprintf(`{"accept":[{"name":%q,"kind":%q,"edited":%t,
		"sources":[{"service_instance_id":%d,"container_ref":%q,"container_name":"Alpha"}]}]}`,
		name, kind, edited, instanceID, ref)
}

// THE HEADLINE FOR THE WRITE: a proposal becomes a library, and the response says
// which of §17.8's two outcomes it was.
func TestAcceptLibrariesCreatesALibraryAndFilesItsWorks(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	code, body := callAccept(t, s, true, acceptBody("Alpha comics", "comic", false, 1, "c-a"))
	if code != http.StatusOK {
		t.Fatalf("POST = %d: %s", code, body)
	}
	var got acceptLibrariesResponse
	mustJSON(t, body, &got)
	if len(got.Items) != 1 {
		t.Fatalf("accepted %d libraries, want 1: %s", len(got.Items), body)
	}
	lib := got.Items[0]
	if lib.Outcome != wireAcceptCreated {
		t.Errorf("outcome = %q, want %q", lib.Outcome, wireAcceptCreated)
	}
	if lib.Name != "Alpha comics" || lib.Slug != "alpha-comics" || lib.Kind != "comic" {
		t.Errorf("accepted %+v, want the name, slug and kind that were sent", lib)
	}
	if lib.MembersFiled != 2 {
		t.Errorf("members_filed = %d, want the two comics c-a holds", lib.MembersFiled)
	}
	if lib.ID == 0 {
		t.Errorf("the accepted library has no id, so the screen cannot link to it")
	}

	// And the proposal is gone from the read, because the container is now bound
	// at its own kind. The two endpoints are one screen and this is the thing
	// that makes them agree.
	_, after := callProposals(t, s, true)
	for _, ref := range proposalRefsOnTheWire(t, after) {
		if ref == "1/c-a" {
			t.Errorf("c-a is still proposed after it was accepted: %s", after)
		}
	}
}

// §17.8's merge rule reaches the wire as its own word: accepting a name that is
// already held AT THE SAME KIND joins rather than creates, and the response says
// so, because the screen has to — *"Joining Kavita Manga into Comics as a second
// source."*
func TestAcceptLibrariesJoinsAnExistingLibrary(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	// `beta` rather than `Beta`: the merge key is case-insensitive and
	// whitespace-trimmed, so this must still join library 200.
	code, body := callAccept(t, s, true, acceptBody("  beta ", "book", false, 1, "c-b"))
	if code != http.StatusOK {
		t.Fatalf("POST = %d: %s", code, body)
	}
	var got acceptLibrariesResponse
	mustJSON(t, body, &got)
	if got.Items[0].Outcome != wireAcceptJoined {
		t.Errorf("outcome = %q, want %q: %s", got.Items[0].Outcome, wireAcceptJoined, body)
	}
	if got.Items[0].ID != 200 {
		t.Errorf("joined library %d, want 200", got.Items[0].ID)
	}
	// NOTHING about the joined row is rewritten — §17.8's one-way door runs in
	// this direction — so the name is the row's, not the acceptance's.
	if got.Items[0].Name != "Beta" {
		t.Errorf("name = %q; joining rewrote the existing library's name", got.Items[0].Name)
	}
}

// THE ONE-WAY DOOR, and the vocabulary translation that carries it. §17.8:
// *"Editing any proposal marks that library user-managed."* The wire says
// `edited`; the column says `user`; managedBy is the only place the two meet.
func TestAcceptLibrariesTranslatesEditedIntoTheStoredColumn(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	for _, tc := range []struct {
		edited bool
		name   string
		want   string
	}{
		{edited: true, name: "Alpha comics", want: "user"},
		{edited: false, name: "Untouched", want: "auto"},
	} {
		ref := "c-a"
		if !tc.edited {
			ref = "c-b"
		}
		kind := "comic"
		if !tc.edited {
			kind = "book"
		}
		code, body := callAccept(t, s, true, acceptBody(tc.name, kind, tc.edited, 1, ref))
		if code != http.StatusOK {
			t.Fatalf("POST edited=%t = %d: %s", tc.edited, code, body)
		}
		var got acceptLibrariesResponse
		mustJSON(t, body, &got)

		var managedBy string
		if err := s.store.DB().Read().QueryRowContext(t.Context(),
			`SELECT managed_by FROM library WHERE id = ?`, got.Items[0].ID).Scan(&managedBy); err != nil {
			t.Fatalf("read managed_by: %v", err)
		}
		if managedBy != tc.want {
			t.Errorf("edited=%t stored managed_by=%q, want %q", tc.edited, managedBy, tc.want)
		}
	}
}

// A NAME THIS USER CANNOT HAVE IS A 409 WITH ITS OWN CODE, never a 500 and never
// the generic `conflict`: the fix is specific and typeable.
//
// ⚠️ AND THE BODY CARRIES NONE OF THE STORE'S OWN SENTENCE. The store wraps this
// sentinel with a message naming an internal row id and a `store:` prefix; the
// user needs the name they typed and the action, and the cause belongs in the
// log.
func TestAcceptLibrariesRefusesATakenName(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	// `Beta` is held by library 200 at kind `book`; this accepts it at `comic`,
	// which cannot join.
	code, body := callAccept(t, s, true, acceptBody("Beta", "comic", true, 1, "c-a"))
	if code != http.StatusConflict {
		t.Fatalf("POST = %d, want 409: %s", code, body)
	}
	var got errorBody
	mustJSON(t, body, &got)
	if got.Error != CodeLibraryNameTaken {
		t.Errorf("error = %q, want %q", got.Error, CodeLibraryNameTaken)
	}
	if got.Action == "" {
		t.Errorf("a 409 the user fixes by typing carries no action: %s", body)
	}
	for _, forbidden := range []string{"store:", "library 200", "sqlite", "SQL", "UNIQUE"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("%q reached the browser in an error body: %s", forbidden, body)
		}
	}

	// ALL OR NOTHING: the refused batch wrote nothing, so the proposal is still
	// a proposal.
	_, after := callProposals(t, s, true)
	if !strings.Contains(after, `"container_ref":"c-a"`) {
		t.Errorf("a refused acceptance consumed the proposal anyway: %s", after)
	}
}

// A SOURCE THE SCOPE DOES NOT ADMIT IS A 404, on notFoundOr's reasoning: "does
// not exist" and "exists but is outside your scope" are deliberately
// indistinguishable, because the difference is an existence oracle. This
// endpoint is where that matters most — it takes an instance id from the caller.
func TestAcceptLibrariesRefusesASourceOutsideTheScope(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	code, body := callAccept(t, s, false, acceptBody("Alpha comics", "comic", false, 1, "c-a"))
	if code != http.StatusNotFound {
		t.Fatalf("POST as a caller outside the scope = %d, want 404: %s", code, body)
	}
	var got errorBody
	mustJSON(t, body, &got)
	if got.Error != CodeNotFound {
		t.Errorf("error = %q, want %q", got.Error, CodeNotFound)
	}
	// Nothing in the body says the instance exists, or names it.
	for _, forbidden := range []string{"store:", "Kavita One", "scope", "1"} {
		if strings.Contains(got.Message, forbidden) {
			t.Errorf("the refusal body carries %q, which is an existence oracle: %s",
				forbidden, body)
		}
	}

	// And the owner can do it, so the assertion above is about the scope.
	if code, ownerBody := callAccept(t, s, true,
		acceptBody("Alpha comics", "comic", false, 1, "c-a")); code != http.StatusOK {
		t.Fatalf("the owner is refused too, so this test proves nothing: %d %s", code, ownerBody)
	}
}

// THE TWO REACHABLE 500s, CLOSED — and closed by CLASSIFYING the storage
// failure rather than by copying `library.kind`'s value list into Go.
//
// Both inputs are things a caller can send today: a kind outside the column's
// CHECK, and an instance id naming no row. As the OWNER, whose scope admits every
// id, so the second one is not caught by the scope check ahead of it.
//
// ⚠️ THE BODY-CONTENT ASSERTION IS HALF THE POINT. SQLite's message for a CHECK
// violation quotes the constraint expression itself — the whole permitted value
// list, verbatim — and a 500 body is exactly where that rides out. So the test
// asserts the status AND that the driver's sentence did not come with it.
func TestAcceptLibrariesClassifiesAStorageRefusal(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int
		code ErrorCode
	}{
		{
			name: "a kind the schema does not have",
			body: acceptBody("Banana Shelf", "banana", false, 1, "c-a"),
			want: http.StatusBadRequest,
			code: CodeBadRequest,
		},
		{
			// 404 rather than 400: it is the same answer as an instance the
			// caller may not see, deliberately, so the pair is not an oracle.
			name: "an instance id naming no row",
			body: acceptBody("Ghost Source", "comic", false, 987654, "c-a"),
			want: http.StatusNotFound,
			code: CodeNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, nil)
			seedProposalsScreenCorpus(t, s)

			code, body := callAccept(t, s, true, tc.body)
			if code != tc.want {
				t.Fatalf("POST = %d, want %d: %s", code, tc.want, body)
			}
			var got errorBody
			mustJSON(t, body, &got)
			if got.Error != tc.code {
				t.Errorf("error = %q, want %q", got.Error, tc.code)
			}
			// The driver's own words, and the schema's, in our error body.
			for _, forbidden := range []string{
				"store:", "sqlite", "CHECK", "constraint", "FOREIGN KEY", "banana", "987654",
			} {
				if strings.Contains(body, forbidden) {
					t.Errorf("%q reached the browser in an error body: %s", forbidden, body)
				}
			}

			// ALL OR NOTHING held: the proposal is still a proposal.
			_, after := callProposals(t, s, true)
			if !strings.Contains(after, `"container_ref":"c-a"`) {
				t.Errorf("a refused acceptance consumed the proposal anyway: %s", after)
			}
		})
	}
}

// The request-shape refusals are 400s naming which acceptance was refused. Each
// is a fact about the DOCUMENT, decided before the store is called, so none of
// them can half-write a batch.
func TestAcceptLibrariesRefusesAMalformedRequest(t *testing.T) {
	s := newTestServer(t, nil)
	seedProposalsScreenCorpus(t, s)

	src := `{"service_instance_id":1,"container_ref":"c-a","container_name":"Alpha"}`
	for _, tc := range []struct{ name, body string }{
		{"an empty batch", `{"accept":[]}`},
		{"no accept key at all", `{}`},
		{"a nameless acceptance", `{"accept":[{"name":"  ","kind":"comic","sources":[` + src + `]}]}`},
		{"a kindless acceptance", `{"accept":[{"name":"Alpha","sources":[` + src + `]}]}`},
		{"an acceptance with no source", `{"accept":[{"name":"Alpha","kind":"comic","sources":[]}]}`},
		{"a source with no service", `{"accept":[{"name":"Alpha","kind":"comic","sources":[` +
			`{"service_instance_id":0,"container_ref":"c-a"}]}]}`},
		{"a source with no container", `{"accept":[{"name":"Alpha","kind":"comic","sources":[` +
			`{"service_instance_id":1,"container_ref":""}]}]}`},
		{"a name past the wire bound", `{"accept":[{"name":"` + strings.Repeat("x", 201) +
			`","kind":"comic","sources":[` + src + `]}]}`},
		{"an unknown field", `{"accept":[{"name":"Alpha","kind":"comic","managed_by":"user",` +
			`"sources":[` + src + `]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := callAccept(t, s, true, tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("POST = %d, want 400: %s", code, body)
			}
			var got errorBody
			mustJSON(t, body, &got)
			if got.Error != CodeBadRequest {
				t.Errorf("error = %q, want %q", got.Error, CodeBadRequest)
			}
		})
	}

	// Nothing was written by any of them.
	var libraries int64
	if err := s.store.DB().Read().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM library WHERE id NOT IN (0, 100, 200, 300)`).
		Scan(&libraries); err != nil {
		t.Fatalf("count libraries: %v", err)
	}
	if libraries != 0 {
		t.Errorf("a refused request created %d libraries", libraries)
	}
}

// The store documents Created and Joined as exclusive with exactly one true. A
// result that is neither is a 500 the log names, never a body that says
// `created` because it was the first branch.
func TestAcceptOutcomeRefusesAResultThatIsNeither(t *testing.T) {
	for _, tc := range []struct {
		name string
		lib  store.AcceptedLibrary
		want string
	}{
		{"created", store.AcceptedLibrary{Created: true}, wireAcceptCreated},
		{"joined", store.AcceptedLibrary{Joined: true}, wireAcceptJoined},
	} {
		got, err := acceptOutcome(tc.lib)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: outcome = %q, want %q", tc.name, got, tc.want)
		}
	}

	for _, lib := range []store.AcceptedLibrary{
		{},
		{Created: true, Joined: true},
	} {
		got, err := acceptOutcome(lib)
		if err == nil {
			t.Errorf("acceptOutcome(%+v) = %q with no error; an impossible pair reached the wire",
				lib, got)
		}
	}
}

// The routes are registered, gated, and reachable at the paths clients will use
// — asserted through the real mux rather than by calling the handlers, which is
// what a route-table typo would slip past.
func TestLibraryProposalRoutesAreRegisteredAndGated(t *testing.T) {
	s := newTestServer(t, nil)
	s.MarkListening()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	if code, _ := get(t, srv.URL+"/api/v1/libraries/proposals"); code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/libraries/proposals unauthenticated = %d, want 401", code)
	}

	// The write is CSRF-gated ahead of the session, and the media-type check is
	// ahead of both — imports_test.go's ordering, asserted here so this route
	// cannot be the one that is gated more weakly than its neighbours. Through
	// that file's cookieClient, so the two routes are measured by one harness.
	path := srv.URL + "/api/v1/libraries/accept"
	c := &cookieClient{t: t, jar: map[string]string{}}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	if code, body := c.do(req); code != http.StatusUnsupportedMediaType {
		t.Errorf("POST %s as text/plain = %d %s, want 415", path, code, body)
	}

	if code, body := c.post(path, false); code != http.StatusForbidden ||
		!strings.Contains(body, `"error":"csrf"`) {
		t.Errorf("POST %s with no CSRF token = %d %s, want 403 csrf", path, code, body)
	}
}
