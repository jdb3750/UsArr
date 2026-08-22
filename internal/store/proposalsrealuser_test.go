package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// §17.8's Accept path, exercised at a user id A SESSION CAN ACTUALLY CARRY.
//
// ⚠️ WHY THIS FILE EXISTS AT ALL, AND WHY THE ID IS THE WHOLE POINT OF IT. Every
// other AcceptLibraries test in this package runs at user 0. No session can ever
// carry user 0: migration 00001 seeds it as `_system` with `is_disabled = 1` and
// no password_hash, and describes it in its own comment as a row that *"can
// never be logged into"*. A real session's id is therefore always >= 1.
//
// That made the whole corpus blind to the defect these tests are about. The
// libraries an install already has sit at user_id 0 — cmd/usarr/import.go builds
// its Importer at store.SystemUserID — so a test that ALSO accepts at user 0 has
// its acting user and its pre-existing data on the same side of every user
// predicate, and cannot tell strict equality (`user_id = ?`) apart from the
// scope predicate (`user_id IN (0, :uid)`). Those two disagree only when the
// acting id is not 0, which is to say: only in production. Running at 7 is what
// makes the two predicates distinguishable, and it is why each test below says
// so where it seeds the user.
//
// ADR-0048 clause 4 is what makes the JOIN the right answer rather than a
// convenient one: it DECLARES the pre-existing `managed_by = 'auto'` rows
// accepted on upgrade, so they are libraries the user already holds.
const realSessionUser int64 = 7

// seedRealSessionUser inserts a user row a session could be issued for.
//
// It is a real row rather than a bare id because `library.user_id` is a foreign
// key onto `user(id)`: accepting at an id with no user row fails on the key
// rather than on the behaviour under test, which would make these tests pass or
// fail for the wrong reason.
func seedRealSessionUser(t *testing.T, s *Store) {
	t.Helper()
	if err := s.DB().Write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (id, username, display_name, auth_source, is_owner, is_disabled)
			VALUES (?, 'owner', 'Owner', 'local', 1, 0)`, realSessionUser)
		return err
	}); err != nil {
		t.Fatalf("seed a real session user: %v", err)
	}
}

// HEADLINE BEHAVIOUR — THE JOIN. This is the defect itself, and before the
// lookup used the scope's predicate this test created a second library instead.
func TestAcceptAtARealSessionJoinsALibraryTheImportLeftAtUserZero(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedRealSessionUser(t, s)

	// Library 100 is the corpus's pre-existing row and it sits at user_id 0,
	// which is where an install's auto-created libraries actually live.
	if owner := libraryOwner(t, s, 100); owner != SystemUserID {
		t.Fatalf("fixture library 100 is owned by %d, want the system sentinel %d — this test "+
			"is about joining a row the IMPORT created, and the import runs at the sentinel",
			owner, SystemUserID)
	}
	before := count(t, s, `SELECT COUNT(*) FROM library`)

	got, err := s.AcceptLibraries(t.Context(), OwnerScope(realSessionUser),
		[]LibraryAcceptance{acceptance("Existing Comics", "comic", source(2, "c-z"))})
	if err != nil {
		t.Fatalf("AcceptLibraries at user %d: %v", realSessionUser, err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if !got[0].Joined || got[0].Created {
		t.Errorf("outcome was created=%v joined=%v, want a JOIN: at user %d the acceptance names "+
			"a library that already exists at user_id 0, and ADR-0048 clause 4 declares it accepted",
			got[0].Created, got[0].Joined, realSessionUser)
	}
	if got[0].ID != 100 {
		t.Errorf("joined library %d, want the existing 100", got[0].ID)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library`); n != before {
		t.Errorf("library rows went %d → %d: the acceptance created a DUPLICATE of a library it "+
			"should have joined", before, n)
	}
	// The duplicate this defect produced was indistinguishable on the wire from
	// the row it duplicated, which is what made `?lib=<slug>` resolve to two ids.
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE slug = 'existing-comics'`); n != 1 {
		t.Errorf("%d libraries carry the slug 'existing-comics', want 1 — two rows with one slug "+
			"is what LibraryIDsBySlug resolves to both of", n)
	}
	// The join must not move the row's ownership: seeing a shared row is not
	// owning it, and nothing about a joined library is rewritten.
	if owner := libraryOwner(t, s, 100); owner != SystemUserID {
		t.Errorf("library 100 is now owned by %d, want it left at %d — a join rewrites nothing "+
			"about the row it joins", owner, SystemUserID)
	}
}

// HEADLINE BEHAVIOUR — THE JOIN PREDICTION. The screen promises the join before
// the user clicks, so the prediction has to be computed from the same population
// the acceptance will resolve against.
func TestProposalsPredictTheJoinAtARealSession(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedRealSessionUser(t, s)

	// Case and padding differ from library 100's name, which libraryNameKey
	// normalises away; the kind matches, which §17.8's merge key also requires.
	observe(t, s, 1, CatalogueContainer{RemoteID: "c-a", Name: "  existing COMICS  ", Kind: "comic"})

	got, err := s.ProposedContainers(t.Context(), OwnerScope(realSessionUser))
	if err != nil {
		t.Fatalf("ProposedContainers at user %d: %v", realSessionUser, err)
	}
	p := proposalFor(t, got, 1, "c-a")
	if p.JoinsLibraryID != 100 {
		t.Errorf("JoinsLibraryID = %d, want 100: at user %d the prediction must see the user_id 0 "+
			"library that AcceptLibraries will join, or the screen promises a create and "+
			"performs a join", p.JoinsLibraryID, realSessionUser)
	}
	if p.JoinsLibraryName != "Existing Comics" {
		t.Errorf("JoinsLibraryName = %q, want %q", p.JoinsLibraryName, "Existing Comics")
	}
}

// HEADLINE BEHAVIOUR — THE NAME REFUSAL. Same name key, different kind: it
// cannot join, and it must not create a second row under the same name either.
func TestAcceptAtARealSessionRefusesANameHeldAtAnotherKind(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedRealSessionUser(t, s)
	before := count(t, s, `SELECT COUNT(*) FROM library`)

	_, err := s.AcceptLibraries(t.Context(), OwnerScope(realSessionUser),
		[]LibraryAcceptance{acceptance("Existing Comics", "book", source(1, "c-a"))})
	if !errors.Is(err, ErrLibraryNameTakenAtOtherKind) {
		t.Fatalf("err = %v, want ErrLibraryNameTakenAtOtherKind: the name is held by library 100 "+
			"at kind comic, and at user %d that row is still one this user holds", err, realSessionUser)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library`); n != before {
		t.Errorf("library rows went %d → %d on a refused accept", before, n)
	}
}

// HEADLINE BEHAVIOUR — THE RESERVED-UNFILED REFUSAL. ⚠️ This is the one the
// defect turned into DEAD CODE: library 0 is `Unfiled` at user_id 0, so under
// strict equality a real session could not see it, the name read as free, and
// the acceptance created a second library called `Unfiled`.
func TestAcceptAtARealSessionRefusesTheReservedUnfiledName(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedRealSessionUser(t, s)
	before := count(t, s, `SELECT COUNT(*) FROM library`)

	// Lower-cased on purpose: the merge key is case-insensitive and
	// whitespace-trimmed, so the refusal must not be escapable by typing it
	// differently from the seeded row.
	_, err := s.AcceptLibraries(t.Context(), OwnerScope(realSessionUser),
		[]LibraryAcceptance{acceptance("  unfiled  ", "movie", source(1, "c-a"))})
	if !errors.Is(err, ErrLibraryNameTakenAtOtherKind) {
		t.Fatalf("err = %v, want the reserved-name refusal: library 0 is `Unfiled` at user_id 0, "+
			"and a session at user %d must still be refused it", err, realSessionUser)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library`); n != before {
		t.Errorf("library rows went %d → %d: a second `Unfiled` was created", before, n)
	}
}

// ONE CONTAINER MAY NOT FEED TWO LIBRARIES OF THE SAME KIND (ADR-0066 decision
// 5), and Accept is the only path that can create that state.
func TestAcceptRefusesOneContainerIntoTwoLibrariesOfTheSameKind(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedRealSessionUser(t, s)

	// Container c-z already feeds library 100, a `comic` library.
	_, err := s.AcceptLibraries(t.Context(), OwnerScope(realSessionUser),
		[]LibraryAcceptance{acceptance("A Second Comics", "comic", source(2, "c-z"))})
	if !errors.Is(err, ErrContainerBoundAtSameKind) {
		t.Fatalf("err = %v, want ErrContainerBoundAtSameKind", err)
	}
	if n := count(t, s, `SELECT COUNT(*) FROM library WHERE name = 'A Second Comics'`); n != 0 {
		t.Errorf("the refused library was created anyway (%d rows)", n)
	}

	// ⚠️ THE SAME REF AT A DIFFERENT KIND IS THE CASE ADR-0066 DECISION 5
	// LICENSES, and it must still be allowed — the refusal above must not be a
	// blanket ban on two libraries over one container.
	if _, err := s.AcceptLibraries(t.Context(), OwnerScope(realSessionUser),
		[]LibraryAcceptance{acceptance("Books From Z", "book", source(2, "c-z"))}); err != nil {
		t.Fatalf("accepting container c-z at kind book: %v — ADR-0066 decision 5 licenses a "+
			"`book` library and a `comic` library over one container ref", err)
	}
}

// A re-accept of a proposal already accepted is a no-op JOIN, not a same-kind
// conflict with itself. This is the case refuseSameKindDoubleBind's `l.id <> ?`
// exists for, and without it the second accept below would refuse.
func TestReAcceptingTheSameProposalIsAJoinNotAConflict(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedRealSessionUser(t, s)

	accepts := []LibraryAcceptance{acceptance("Fresh Books", "book", source(1, "c-a"))}
	first, err := s.AcceptLibraries(t.Context(), OwnerScope(realSessionUser), accepts)
	if err != nil {
		t.Fatalf("first accept: %v", err)
	}
	if !first[0].Created {
		t.Fatalf("first accept did not create: %+v", first[0])
	}
	second, err := s.AcceptLibraries(t.Context(), OwnerScope(realSessionUser), accepts)
	if err != nil {
		t.Fatalf("re-accept: %v — the same container into the SAME library is a no-op, not a "+
			"same-kind double bind", err)
	}
	if !second[0].Joined || second[0].ID != first[0].ID {
		t.Errorf("re-accept gave %+v, want a join of library %d", second[0], first[0].ID)
	}
}

// A library created at a real session belongs to that session's user, not to
// the sentinel it can read. This is the write half of the read/write asymmetry.
func TestAcceptAtARealSessionWritesTheActingUsersID(t *testing.T) {
	s := newTestStore(t)
	seedProposalsCorpus(t, s)
	seedRealSessionUser(t, s)

	got, err := s.AcceptLibraries(t.Context(), OwnerScope(realSessionUser),
		[]LibraryAcceptance{acceptance("Fresh Books", "book", source(1, "c-a"))})
	if err != nil {
		t.Fatalf("AcceptLibraries: %v", err)
	}
	if owner := libraryOwner(t, s, got[0].ID); owner != realSessionUser {
		t.Errorf("library %d is owned by %d, want the acting user %d: the lookup reads "+
			"`user_id IN (0, :uid)` but the INSERT writes the acting user alone",
			got[0].ID, owner, realSessionUser)
	}
}

func libraryOwner(t *testing.T, s *Store, libraryID int64) int64 {
	t.Helper()
	var owner int64
	if err := s.DB().Read().QueryRowContext(t.Context(),
		`SELECT user_id FROM library WHERE id = ?`, libraryID).Scan(&owner); err != nil {
		t.Fatalf("read owner of library %d: %v", libraryID, err)
	}
	return owner
}
