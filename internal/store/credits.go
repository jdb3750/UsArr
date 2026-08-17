package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// The credit write path: how an author becomes a row (ADR-0044, migration 0007).
//
// A CREDIT IS TWO WRITES, NOT ONE COLUMN, and that is the cost the owner
// accepted when he approved authors into v0.1. `work_credit.creator_work_id` is
// a foreign key into `work`, so every author, translator, penciller or letterer
// is a `work` row of kind 'person' before it is a credit. This file creates
// both.
//
// WHAT A 'person' WORK DELIBERATELY DOES NOT GET, because each omission is a
// rule from docs/reference/schema.md §6.1 rather than an oversight:
//
//   - NO search_doc / search_fts / search_trgm ROW. 'person' is excluded from
//     the FTS corpus because there is no person screen in any milestone, so a
//     person hit would be a search result with nowhere to land. This path never
//     calls rebuildSearchDoc, and TestPeopleNeverEnterTheSearchCorpus is the
//     assertion — fired deliberately against a builder that did index them.
//   - NO library_member ROW. A person is not a catalogue item and belongs to no
//     library. Nothing files one, which also means no search_doc_library row
//     could exist for one even if a doc did.
//   - NO SUBTYPE TABLE. §6.1 gives 'person' an explicit justification for having
//     none; TestMigrate0007NoWorkPersonTable pins the absence.
//   - NO external_id ROW. See personWorkID below for why the obvious candidate —
//     Kavita's own person id — is the wrong thing to write.
//
// It is a replication write and takes no Scope, on catalogue.go's header's
// reasoning: it is driven by a background importer with no acting user.

// Credit is one credited creator on one work, as an adapter projected it.
//
// Like CatalogueItem this is NOT an upstream DTO: internal/store never sees a
// kavita.PersonDto, and which of Kavita's thirteen person arrays becomes which
// role is the adapter's decision, made in internal/libsync/credits.go.
type Credit struct {
	// Role is a member of work_credit.role's CHECK. A non-member is rejected by
	// the database rather than by this package: the CHECK is the vocabulary's
	// single definition and a Go-side copy would be a second one to drift.
	Role string

	// Name is the creator's name as the upstream printed it. It becomes the
	// person work's title AND its sort_title — deriving "Vaughan, Brian K."
	// from "Brian K. Vaughan" is exactly the parse ARCHITECTURE §6.5 rule 3
	// forbids, and getting it wrong on a mononym, a Japanese name or a studio
	// pseudonym is the ordinary case rather than the edge one.
	Name string

	// NormalizedName is NormalizeTitle(Name), computed by the adapter. IT IS THE
	// DEDUPE KEY — see personWorkID.
	NormalizedName string

	// Position is billing order within (work, role).
	Position int

	// CreditedAs is the string this release printed when it differs from the
	// person work's title. Kavita reports one name per person and no per-series
	// variant, so its adapter always leaves this empty; the column exists for
	// the sources that do report one.
	CreditedAs string
}

// CreditSet is every credit one already-replicated item carries.
//
// It is keyed by the UPSTREAM's identifiers rather than by a work id, because
// the credit pass runs after the item stream has closed and re-resolves through
// service_item_link — the same row ApplyCatalogueBatch wrote. Carrying a work id
// across the two passes would mean trusting an id read in a different
// transaction, and an item deleted between the two passes would then have its
// credits written onto whatever now holds that id.
type CreditSet struct {
	RemoteKind string
	RemoteID   string
	Credits    []Credit
}

// CreditResult is what one ApplyCredits call did.
type CreditResult struct {
	// PeopleCreated counts new 'person' works. PeopleReused counts credits that
	// landed on a person work that already existed — across this batch, this
	// import and every previous import alike, because the dedupe is a database
	// lookup and not a per-run cache.
	PeopleCreated int
	PeopleReused  int

	CreditsWritten int

	// ItemsUnresolved counts sets whose remote item has no service_item_link.
	// On a healthy import that is zero; it is non-zero when the item stream
	// failed part-way and the credit pass still ran over what it read.
	ItemsUnresolved int

	// CreditsRejected counts credits the database refused — in practice a role
	// that is not a member of work_credit.role's CHECK. The credit is dropped,
	// the rest of the set stands, and the count is what the report surfaces.
	CreditsRejected int
}

func (r *CreditResult) add(o CreditResult) {
	r.PeopleCreated += o.PeopleCreated
	r.PeopleReused += o.PeopleReused
	r.CreditsWritten += o.CreditsWritten
	r.ItemsUnresolved += o.ItemsUnresolved
	r.CreditsRejected += o.CreditsRejected
}

// Add merges another result into r, so an importer can accumulate across
// batches without reimplementing the sum.
func (r *CreditResult) Add(o CreditResult) { r.add(o) }

// ApplyCredits writes one batch of credit sets in ONE BEGIN IMMEDIATE
// transaction, on ApplyCatalogueBatch's sizing and for its reasons.
//
// It is a SEPARATE CALL FROM ApplyCatalogueBatch rather than a step inside it,
// and the reason is the shape of the upstream rather than tidiness: Kavita
// reports no creator on the series list at all, so credits cost one extra HTTP
// GET per series (kavita.SeriesMetadataDto explains why there is no bulk form).
// Making that call from inside the item stream's callback would hold the
// streaming connection open across N round trips to the same server. So the
// importer runs this as a phase-B pass after the stream closes — which is
// reference/sync.md §2's own two-phase rule, with credits in phase B beside
// overview and file details.
func (s *Store) ApplyCredits(
	ctx context.Context, instanceID int64, sets []CreditSet, now time.Time,
) (CreditResult, error) {
	var res CreditResult
	if len(sets) == 0 {
		return res, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res = CreditResult{}
		// people caches the (normalized name → person work id) resolutions this
		// transaction has already made. It is an optimisation and NOT the
		// dedupe: personWorkID's database lookup is what makes two books by one
		// author share a row across separate imports, separate batches and a
		// restarted process. A cache-only dedupe would be correct exactly until
		// the second batch.
		people := make(map[string]int64)
		for _, set := range sets {
			one, err := applyOneCreditSet(ctx, tx, instanceID, set, people, now)
			if err != nil {
				return fmt.Errorf("apply credits for %s %s from service_instance %d: %w",
					set.RemoteKind, set.RemoteID, instanceID, err)
			}
			res.add(one)
		}
		return nil
	})
	return res, err
}

func applyOneCreditSet(
	ctx context.Context, tx *sql.Tx, instanceID int64,
	set CreditSet, people map[string]int64, now time.Time,
) (CreditResult, error) {
	var res CreditResult

	// ── 1. Re-resolve the work through the link the item pass wrote.
	var workID int64
	err := tx.QueryRowContext(ctx, `
		SELECT work_id FROM service_item_link
		 WHERE service_instance_id = ? AND remote_kind = ? AND remote_id = ?
		   AND deleted_at IS NULL`,
		instanceID, set.RemoteKind, set.RemoteID).Scan(&workID)
	if errors.Is(err, sql.ErrNoRows) {
		res.ItemsUnresolved++
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("look up link: %w", err)
	}

	// ── 2. Replace this work's credits wholesale, for work_alt_title's reason:
	// an upstream that DROPS a credit must not leave it standing forever. A
	// diff would be cheaper and would also silently keep a penciller the
	// operator deleted in Kavita last week.
	//
	// It deletes only credits of THIS work. A person credited on other works
	// keeps those, which is what ix_credit_creator's second reader is for.
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_credit WHERE work_id = ?`, workID); err != nil {
		return res, fmt.Errorf("clear credits of work %d: %w", workID, err)
	}

	for _, c := range set.Credits {
		if c.Name == "" || c.NormalizedName == "" {
			// A creator with no name is nothing to credit. Kavita's PersonDto
			// declares `name` required and nullable in the same schema, so an
			// empty one is reachable.
			continue
		}
		creatorID, created, err := personWorkID(ctx, tx, c, people, now)
		if err != nil {
			return res, err
		}
		if created {
			res.PeopleCreated++
		} else {
			res.PeopleReused++
		}

		// ON CONFLICT (…) DO NOTHING, and specifically NOT `INSERT OR IGNORE`.
		// The two look interchangeable and are not: OR IGNORE swallows EVERY
		// constraint failure including the CHECK, so an adapter emitting a role
		// that is not a member of the vocabulary would be silently dropped with
		// nothing to count. Naming the conflict target scopes the tolerance to
		// UNIQUENESS, which is the one conflict that is a legitimate upstream
		// condition: a source that reports the same person twice in one role
		// array — Kavita does, when two chapters of one series credit the same
		// writer with different capitalisation — sends two credits that
		// normalise to one person at one position, and the second is the SAME
		// FACT rather than an error. Measured, not assumed:
		// TestAnIllegalRoleIsDroppedNotFatal was written against OR IGNORE
		// first and read CreditsRejected = 0.
		r, err := tx.ExecContext(ctx, `
			INSERT INTO work_credit
			  (work_id, creator_work_id, role, position, credited_as)
			VALUES (?,?,?,?,?)
			ON CONFLICT (work_id, role, position, creator_work_id) DO NOTHING`,
			workID, creatorID, c.Role, c.Position, nullString(c.CreditedAs))
		if err != nil {
			// The realistic cause is work_credit.role's CHECK. The credit is
			// dropped and counted rather than aborting the batch: one bad role
			// from one adapter must not cost 2,000 items their credits.
			//
			// NO SAVEPOINT, and that is checked rather than assumed. SQLite's
			// default conflict resolution is ABORT, which undoes the failing
			// STATEMENT and leaves the transaction usable — unlike ROLLBACK,
			// which would kill it. catalogue.go fences its external_id write
			// with a SAVEPOINT because that path must also READ the conflicting
			// row afterwards; this one has nothing to read. The transaction's
			// survival is asserted by execution: the two legal credits either
			// side of the illegal one are still there at the end of
			// TestAnIllegalRoleIsDroppedNotFatal.
			res.CreditsRejected++
			continue
		}
		n, err := r.RowsAffected()
		if err != nil {
			return res, fmt.Errorf("credit %q on work %d: %w", c.Role, workID, err)
		}
		if n > 0 {
			res.CreditsWritten++
		}
	}
	return res, nil
}

// personWorkID resolves a credited name onto a `work` of kind 'person',
// creating one only when no existing person matches.
//
// # The dedupe key is the NORMALIZED NAME, and what that buys and costs
//
// The key is `work.normalized_title` under `kind = 'person'` — the same v1
// normaliser every other title in the schema is keyed by (casefold, NFKD, strip
// combining marks, strip punctuation, collapse whitespace), so "Brian K.
// Vaughan", "brian k vaughan" and "Brian  K.  Vaughan" are one person, and
// `ix_work_norm(normalized_title, year, kind)` serves the lookup as a seek.
// **Two books by the same author are therefore one person work, not two** —
// which is the property TestTwoWorksByOneAuthorShareOnePersonWork fires against.
//
// ⚠️ IT IS WRONG IN BOTH DIRECTIONS AND BOTH ARE CARRIED RATHER THAN HIDDEN:
//
//   - A NAME VARIANT SPLITS. "A. Moore" and "Alan Moore" normalise differently
//     and become two person works, each credited on its own books. Nothing
//     merges them, and in v0.1 nothing surfaces that they are the same human —
//     'person' has no screen. The seam for fixing it exists and is not built:
//     kavita.PersonDto carries an `aliases` array, and folding aliases into
//     work_alt_title rows on the person work would let a later matcher resolve
//     both to one row. That is a matcher, not a column, so it waits.
//   - TWO PEOPLE WITH ONE NAME MERGE. Two different John Smiths become one
//     person work credited on both their books. This is the direction that
//     actually loses information, and it is accepted on the same ground
//     ARCHITECTURE §6.4 rule 1 uses for works — no source UsArr reads gives a
//     person a stable, global identifier, so the alternative is not "a better
//     key", it is "a person work per credit per series", which makes the table
//     a list of strings with foreign keys.
//
// # Why Kavita's own person id is NOT the key
//
// kavita.PersonDto.Id is the obvious candidate and it is wrong twice over. It is
// INSTANCE-LOCAL, so writing it as an external_id row means two Kavita installs
// both claiming person 5 — and `ux_extid_work_strong` (UNIQUE(source, value)
// WHERE work_id IS NOT NULL AND confidence >= 1.0) reads that collision as a
// MERGE SIGNAL between two unrelated humans, which is worse than the name
// collision it was meant to fix. And it is not stable across a Kavita library
// rescan of a series whose ComicInfo changed. So no external_id row is written
// for a person at all, and the name is the whole key.
//
// # Soft deletes
//
// The lookup excludes soft-deleted people, so a person inside the 7-day
// tombstone window is not resurrected by a credit; a new row is created and the
// tombstone expires on its own. That is the same call the work path makes.
func personWorkID(
	ctx context.Context, tx *sql.Tx, c Credit, people map[string]int64, now time.Time,
) (id int64, created bool, err error) {
	if id, ok := people[c.NormalizedName]; ok {
		return id, false, nil
	}

	err = tx.QueryRowContext(ctx, `
		SELECT id FROM work
		 WHERE kind = 'person' AND normalized_title = ? AND deleted_at IS NULL
		 ORDER BY id LIMIT 1`, c.NormalizedName).Scan(&id)
	switch {
	case err == nil:
		people[c.NormalizedName] = id
		return id, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, false, fmt.Errorf("look up person %q: %w", c.Name, err)
	}

	nowStr := FormatTime(now)
	// sort_title IS the name, unreversed. See Credit.Name.
	//
	// added_at is left NULL: a person was not "added to the library" on any
	// date, and ix_work_added orders the recently-added table, which a person
	// must never appear in.
	r, execErr := tx.ExecContext(ctx, `
		INSERT INTO work (kind, title, sort_title, normalized_title, norm_version,
		                  created_at, updated_at)
		VALUES ('person', ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Name, c.NormalizedName, NormVersionForPeople, nowStr, nowStr)
	if execErr != nil {
		return 0, false, fmt.Errorf("create person %q: %w", c.Name, execErr)
	}
	id, execErr = r.LastInsertId()
	if execErr != nil {
		return 0, false, fmt.Errorf("create person %q: id: %w", c.Name, execErr)
	}
	people[c.NormalizedName] = id
	return id, true, nil
}

// NormVersionForPeople is the norm_version stamped on a 'person' work.
//
// It is the same algorithm version the adapters stamp on every other work, and
// it is named separately here only so the coupling is visible: this package
// cannot import internal/libsync (that package imports this one), so the
// constant is duplicated rather than the dependency inverted, and
// TestPersonNormVersionMatchesTheNormaliser is what keeps the two equal.
const NormVersionForPeople int64 = 1
