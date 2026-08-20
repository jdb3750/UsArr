/**
 * HOME's vocabulary, as pure functions. ARCHITECTURE.md §17.2 (as amended by
 * ADR-0028), §17.7, and DESIGN-DIRECTION §8.4, §9.6, §10.
 *
 * DOM-free and deterministic, so the node-environment vitest run can pin it.
 * The rendering is in `routes/+page.svelte`; the decisions that §17.2 makes
 * RULES rather than taste live here, because a rule inside a `{#if}` in a
 * template is a rule nothing can test.
 *
 * WHICH OF HOME'S THREE BLOCKS THIS FILE SERVES.
 *
 * ADR-0028 fixes Home at three blocks: A, a ≤6-row media-type summary; B, an
 * attention block hidden entirely when empty; C, one unified recently-added
 * table. ⚠️ THIS READ **"A and C have no data source in this build and are
 * therefore not drawn at all"**, and then **"Block A is still not"** once
 * Block C's read landed. **Both are dead.** `GET /api/v1/library/facets` is the
 * second read Block A was blocked on — `internal/httpapi/facets.go`, six counts
 * over the local file — so this file computes Block A (`librarySummary` below)
 * and Block B, and `$lib/library` computes Block C. **Read
 * `web/src/routes/+page.svelte` for what is drawn, never this comment.**
 *
 * DESIGN-DIRECTION §9.6's ban is unchanged and is what shapes the block rather
 * than what suppresses it: never fabricated data in a shipped product surface,
 * so the section does not render before its read lands (no skeleton, no zeroed
 * table), and it draws only the two columns the wire answers. §17.2's `Have`
 * and `Synced` are *"their own aggregates and their own commit"*
 * (`internal/httpapi/facets.go`) and are therefore not drawn at all.
 *
 * ⚠️ TWO THINGS ON HOME ARE NOT ONE OF THE THREE BLOCKS, and saying so here is
 * the point rather than a caveat. A release-search entry point and the recent
 * grabs list both have real sources in this build and neither is Block A or
 * Block C. ⚠️ THE REASON GIVEN FOR THAT WAS **"`Recently added` is a catalogue
 * read that does not exist yet"**, AND THAT PREMISE IS DEAD WITH THE ONE ABOVE:
 * the read exists and Block C is drawn off it. **The conclusion does not move,
 * and it now rests on something firmer than what it was given.** While Block C
 * was undrawn, filing recent grabs under its heading was an IMPRECISION — a
 * claim about a catalogue that no catalogue on the screen could contradict.
 * Block C exists now, so that heading names a REGION THAT IS ALREADY THERE
 * MEANING SOMETHING ELSE: `Recently added` is the CATALOGUE ordered by
 * `added_at`, while a grab is a release handed to a download client, which
 * UsArr stops watching at the moment Prowlarr accepts it (`$lib/requests`'
 * KNOWLEDGE_STOPS_NOTE). The two claims would COLLIDE rather than merely
 * overstate — one form of words over two different facts, both on screen at
 * once — which is the invented status the rest of this file is written against.
 * What this module adds for them is the two rules that are rules — whether a
 * search box may be drawn at all (`hasIndexer`) and what it must say it
 * searches (`HOME_SEARCH_SCOPE_NOTE`) — because a rule inside a `{#if}` in a
 * template is a rule nothing can test. The grabs themselves are
 * `$lib/requests`' vocabulary, reused rather than restated.
 *
 * §17.2's hard rule is the one an implementer breaks first: a media type the
 * user does not have is not shown AT ALL, not in Block A, not in the sidebar,
 * not as a search group. ⚠️ IT WAS ONCE READ HERE AS AN ARGUMENT FOR DRAWING
 * NO ROWS, on the premise that *"with no catalogue there are no types the user
 * has"*, and that premise died twice over: a connected catalogue source writes
 * `book` and `comic` works, and the per-type read that would count them now
 * exists. **The rule is satisfied by the `unconfigured` STATE rather than by
 * omission** — §17.2 says so itself, and `design/DESIGN-DIRECTION.md` rule 13
 * says why rows that carry a state, a cause and an action are not an empty
 * section. `librarySummary` below is where that lands, and it explains which
 * types this build can catalogue and how that was measured. ⚠️ Rule 13 counts
 * FOUR such rows and this build draws three, which is the audiobooks
 * disagreement `librarySummary` records rather than a second rule.
 *
 * 🕰️ THIS PARAGRAPH ONCE CONTINUED, TRULY OF THE TREE IT WAS WRITTEN AGAINST:
 * *"The sidebar draws none either (`routes/+layout.svelte`, whose `NAV_GROUPS`
 * is the six fixed entries and no media types); Home matches it. THAT IS ONE
 * TASK RATHER THAN TWO … both are blocked on the same missing read, so whoever
 * builds the rollup unblocks Block A and the sidebar together."*
 *
 * ⚠️ IT IS KEPT AS HISTORY AND IT NO LONGER DESCRIBES THIS TREE, and those are
 * two different things. `NAV_GROUPS` now carries `TYPE_NAV`: all six media
 * types, drawn UNCONDITIONALLY, over `/library/[type]`. Read the layout, not
 * this quote, for what the sidebar does.
 *
 * It is kept because ADR-0053 CITES IT — as the record that shipping no type
 * entries at all was a live option someone held and argued for, rather than one
 * nobody thought of. A rejected alternative with no trace of ever having been
 * believed is indistinguishable from an oversight, and deleting the sentence
 * would leave that ADR pointing at nothing. The ADR carries the reasoning and
 * the rejection; do not restate either here. `routes/+layout.svelte` carries the
 * shipped rule at `TYPE_NAV`, and `docs/reference/http-api.md` §7.1 is where the
 * missing facet count is written down.
 *
 * What the quote got WRONG, and why the sidebar could ship without the rollup:
 * the two rows are not the same KIND of thing. A sidebar row is a PLACE, and a
 * link needs no number to be honest; Block A's row IS a number, and until
 * `GET /api/v1/library/facets` landed it had none to render. So the sidebar was
 * never blocked and Block A was, on that one read — which is why the two were
 * never the single task the quote called them.
 *
 * ⚠️ AND `http-api.md` §7.1's *"no facet counts beside the chips"* IS STILL THE
 * SIDEBAR'S RULE. ADR-0053 draws all six entries unconditionally and none of
 * them carries a number; the facet read does NOT discharge that ADR's reopening
 * condition, which ADR-0059 refined to an independent EXISTS over
 * `edition.format` rather than to this count. Nothing here changes the sidebar.
 *
 * WHAT MODE THE SCREEN IS IN IS DERIVED FROM THE API AND NEVER FROM A
 * CONSTANT. §8.5 defines Search-and-Grab mode as "activated when no configured
 * instance advertises `LibrarySync`", and the health endpoint carries the
 * `role` that decides it. So the mode is a function of the response, and a
 * build that later accepts a library-bearing kind changes what this returns
 * without anything here being edited.
 */

import type { ServiceHealth, ServicesHealth } from './api';
import { MEDIA_TYPES, mediaTypeLabel, type MediaType, type MediaTypeCounts } from './library';
import {
	isIndexer,
	needsAttention,
	stateLabel,
	type ServiceRow,
	type StateLabel,
	type Tone
} from './services';

/**
 * Which state §17.7 puts Home in, decided from what is actually configured.
 *
 *   `unconfigured`     nothing is connected. §17.7: this goes to the first-run
 *                      path and NEVER to an empty home page.
 *   `search-and-grab`  services exist and none of them is library-bearing.
 *                      §8.5's named configuration, not an implicit empty app.
 *   `library`          at least one library-bearing service is configured.
 *
 * `library` BECAME REACHABLE when `kavita` → role `library` joined
 * `internal/httpapi.serviceKinds` (ADR-0041), and this comment used to say the
 * opposite — that every instance which could exist was an indexer. The mode was
 * derived rather than assumed away precisely so that this day needed no change
 * here, and it did not.
 *
 * ⚠️ IT WENT ON TO SAY **"what the arm renders is still one honest sentence and
 * NO catalogue: a Kavita can be added and probed, and nothing imports from it
 * yet"**, AND BOTH HALVES OF THAT ARE DEAD. Something imports from it —
 * `internal/libsync`'s Kavita source, run by `cmd/usarr`'s `bootstrapImport`
 * from the `kavita` arm of the client build (`cmd/usarr/services.go`) — and the
 * arm draws Block C's table off `GET /api/v1/library/recent`, not a sentence.
 *
 * The SHAPE of that import is what is still worth stating, because the dead
 * sentence undershot and a bare "the library sync is built" would overshoot by
 * as much: ONE adapter, Kavita and nothing else (`cmd/usarr/import.go` refuses
 * any other kind); ON CONNECT, gated on `last_full_sync_at` being unset, so it
 * runs at most once per instance per database; NO timer and NO periodic
 * re-sync; and the manual `FullImport` has no HTTP route and no CLI flag in
 * front of it, so nothing a user can reach asks for a second import. Read
 * `internal/db/migrations`, `cmd/usarr/import.go` and `web/src/routes` for what
 * exists, never this comment.
 */
export type HomeMode = 'unconfigured' | 'search-and-grab' | 'library';

export function homeMode(health: ServicesHealth): HomeMode {
	if (health.setupRequired || health.services.length === 0) return 'unconfigured';
	// §8.5's own test, applied to the field that carries it. `isIndexer` is the
	// Services screen's existing predicate rather than a second copy of it.
	if (health.services.every(isIndexer)) return 'search-and-grab';
	return 'library';
}

/**
 * WHETHER HOME MAY OFFER A RELEASE SEARCH AT ALL, and it is deliberately NOT
 * `mode === 'search-and-grab'`.
 *
 * The precondition for the box is that something can answer it: at least one
 * configured service is an indexer. That is a narrower test than the mode and a
 * different one — `library` mode means a library-bearing service exists, and an
 * install with a Sonarr AND a Prowlarr is in `library` mode with a perfectly
 * good indexer to search. Gating on the mode would delete a working control on
 * the first build that connects a Sonarr, which is the same class of mistake
 * `homeMode` exists to avoid on the other side.
 *
 * The inverse matters more: on an `unconfigured` install this is false, so Home
 * never draws an input that has nothing behind it. A search box that accepts a
 * query and can only fail is the invented status CLAUDE.md forbids, expressed
 * as a control rather than as a number — the same reason `routes/search` draws
 * a sentence instead of an input.
 *
 * ⚠️ THE REASON GIVEN FOR THAT SENTENCE WAS **"over an index that does not
 * exist"**, AND THAT PREMISE IS DEAD. The index exists: `internal/libsync`
 * imports a catalogue and `internal/store`'s `ApplyCatalogueBatch` writes
 * `search_doc`, `search_fts` and `search_trgm` beside the rows, all three
 * created by `internal/db/migrations/00005_library_sync.sql`. The screen this
 * sentence points AT has since retracted it in as many words
 * (`web/src/routes/search/+page.svelte`, "THE INDEX IS NO LONGER THE MISSING
 * HALF"), so this line was citing a reason its own referent had withdrawn.
 * **The analogy survives on the half that is real**: what is missing is the
 * QUERY. `internal/httpapi/server.go` registers `GET /api/v1/library/recent`
 * and no other `/api/v1/library/*` route, so a library-search input still has
 * nothing to call. Same hole, one layer up from where this used to put it.
 */
export function hasIndexer(health: ServicesHealth): boolean {
	return health.services.some(isIndexer);
}

/**
 * WHAT THE BOX SEARCHES, SAID ON THE BOX.
 *
 * DESIGN-DIRECTION §8.3 keeps two things apart that the *Arrs also keep apart:
 * **library search** — the persistent input, jumping to something you already
 * have — and **release search**, a dedicated screen. *"Merging them is how a
 * 0 ms local query ends up waiting on a 30 s indexer."* Home's box is the FIRST
 * kind, and it says which it is because there is now more than one search in
 * this product and nothing else on the screen distinguishes them.
 *
 * It is also why the box is a ROUTE and not a search field. Nothing on Home
 * queries anything; submitting navigates to `/search` with `?q=`, which is
 * §17.4's own mechanism (`requestsSearchHref`). Home stays a local read.
 *
 * ⚠️ THE SEAM THIS STRING WAS WRITTEN AS HAS NOW BEEN USED, WHICH IS THE ONE
 * THING A SEAM IS FOR. It said: *"§8.3 gives library search a persistent input,
 * at which point Home's box becomes library search: same element, same submit,
 * a different destination and this sentence rewritten. Keeping the destination
 * in `requestsSearchHref` and the scope in a constant is what makes that a
 * two-line change rather than a rebuild."* That is what happened, on the
 * owner's instruction — *"the search on home should be searching your own
 * library"* — and it cost the two lines it said it would: the base passed to
 * `requestsSearchHref` moved from `/requests` to `/search`, and this string was
 * rewritten. What the seam did NOT predict is the third line, and it is worth
 * recording because it is the part a seam cannot carry: the box's own GATE had
 * to move from `hasIndexer` to `mode === 'library'`, since the precondition for
 * a control is a fact about its destination and the destination changed.
 *
 * ⚠️ THE OLD STRING NAMED A CORPUS THIS BOX NO LONGER REACHES. It read:
 * *"This searches releases on your indexers, not your own library. Results, the
 * indexer and category pickers and the grab are on Requests, and pressing
 * Search takes you there."* Every clause of it is still true OF REQUESTS, which
 * is why none of it was deleted from the product, only from here.
 *
 * ⚠️ AND IT DOES NOT POINT AT THE OTHER SEARCH, deliberately. `routes/search`
 * carries a `Search indexers` action on every incomplete result row and another
 * on its zero-results state (§17.4 rules 6 and 7), so a user who wanted
 * indexers is one click from them at the moment they discover they wanted them,
 * which is later than this sentence and better informed. A pointer here would
 * be a third mention of release search on a screen the owner has just said has
 * too many searches on it.
 */
export const HOME_SEARCH_SCOPE_NOTE =
	'This searches the library UsArr has replicated from your services, not your indexers. ' +
	'It renders from SQLite, so it never waits on a service.';

/**
 * One row of Block B. §17.2: the block is the differentiator and no surveyed
 * tool has anything to put in it, and it is **hidden when empty** rather than
 * rendering a green "all good" panel.
 *
 * THE COLUMNS ARE NOT THE SERVICES SCREEN'S COLUMNS, and the difference is
 * §17.3's rule that a problem is stated canonically once per screen. `state`
 * and `detail` are UsArr's own words, straight from `stateLabel()` so the two
 * screens cannot drift; `problem` is the upstream's own words, verbatim, and
 * is the only thing here that is not UsArr speaking. What Block B does NOT
 * carry is the fix: the button that repairs a service lives on the Services
 * row, and Home links to that row instead of growing a second copy of it.
 */
export interface AttentionRow {
	id: number;
	name: string;
	tone: Tone;
	icon: StateLabel['icon'];
	/** UsArr's own plain-language word for the state. Never the mechanism's. */
	state: string;
	/** The qualifying clause under it, or ''. */
	detail: string;
	/** The upstream's own text, VERBATIM after redaction, or ''. */
	problem: string;
}

/**
 * Block B's rows, in the order the services came back in.
 *
 * The membership test is `needsAttention()`, which is the same predicate the
 * Services roll-up uses. Two lists answering "what is wrong" from two
 * predicates is how a screen ends up reporting three problems while another
 * reports four, and the deduction the user makes from that is that one of them
 * is lying.
 */
export function attention(services: readonly ServiceHealth[], now: Date): AttentionRow[] {
	const out: AttentionRow[] = [];
	for (const health of services) {
		const row: ServiceRow = { health };
		const state = stateLabel(row, now);
		if (!needsAttention(row, now)) continue;
		out.push({
			id: health.id,
			name: health.name,
			tone: state.tone,
			icon: state.icon,
			state: state.word,
			detail: state.detail,
			problem: health.problem?.trim() ?? ''
		});
	}
	return out;
}

// Re-exported through this module so the screen imports one vocabulary rather
// than reaching into the Services screen's file for half of it.
export { needsAttention };

/**
 * What the page head says, and it is derived from the same data the blocks
 * are.
 *
 * A CONSTANT HERE IS THE FAILURE THIS FUNCTION EXISTS TO PREVENT, and the
 * mockup records it happening: a fixed "Last delta sync 14:02, 6 minutes ago"
 * sat above "No services configured" on the very first screen a new user saw —
 * UsArr reporting a completed sync over a system with nothing connected, first
 * in reading order and first in the accessibility tree.
 *
 * `count` is the roll-up's count string (`1 error, 2 warnings`) or ''. It is
 * passed in rather than recomputed so the head and the block cannot disagree.
 */
export function headline(mode: HomeMode | undefined, services: number, count: string): string {
	if (mode === undefined) return 'Reading what is connected.';
	if (mode === 'unconfigured') return 'No service is connected yet.';
	const connected = `${services} ${services === 1 ? 'service' : 'services'} connected.`;
	// "Nothing needs attention" is a claim about what was computed, and it is
	// exactly what an empty roll-up means. "Every service is healthy" is a
	// stronger claim than the roll-up makes — it skips an enabled instance
	// whose probe ran and could not classify it — so it is not the sentence.
	const state = count === '' ? 'Nothing needs attention.' : `${count}.`;
	if (mode === 'search-and-grab') return `Search-and-Grab mode. ${connected} ${state}`;
	return `${connected} ${state}`;
}

/* ── BLOCK A: the media-type summary ──────────────────────────────────────── */

/**
 * WHICH MEDIA TYPES A CONNECTED SERVICE KIND CAN WRITE WORKS OF.
 *
 * ⚠️ THIS IS A PROPERTY OF THE INSTALL, NOT OF THE BUILD, AND IT SHIPPED AS THE
 * SECOND ONE. A static table of "types v0.1 catalogues" told a Kavita-only user
 * `Audiobooks · 0 audiobooks`, which is the exact claim `librarySummary` below
 * argues no row may make: a claim about the library where the true fact is
 * about the pipeline. Kavita serves no audio and says so
 * (`internal/libsync/kavita.go`, the `LibraryTypeBook` arm: *"Kavita serves no
 * audio, so nothing here can produce one"*), so on that install the Audiobooks
 * row has no source and must say so.
 *
 * READ OFF `internal/libsync`, KIND BY KIND, never off a document:
 *
 *   kavita     `LibraryTypeBook` → `book` works, and no audio at all, so
 *              EBOOKS; `LibraryTypeComic` / `ComicVine` / `Manga` → `comic`
 *              works, so COMICS.
 *   bookorbit  the same two work kinds plus the audio half: its
 *              `bookOrbitEditionFormat` maps m4b, mp3, m4a, opus, ogg and flac
 *              onto `edition.format = 'audiobook'` (ADR-0059), so EBOOKS,
 *              AUDIOBOOKS and COMICS.
 *
 * `cmd/usarr/import.go` is the bound on this map: it refuses any kind but those
 * two, in as many words, so a kind absent here catalogues nothing whatever its
 * `role` says. A kind that is registered as `library` in
 * `internal/httpapi.serviceKinds` but has no adapter would fall through to no
 * types at all, which is the truthful answer rather than a default.
 *
 * ⚠️ THE SPLIT DISAGREES WITH `design/DESIGN-DIRECTION.md` §8.4, WHICH LISTS
 * AUDIOBOOKS AMONG v0.1's SOURCELESS TYPES on the premise that *"BookOrbit's
 * media types are Kavita's"*. They are not — the audio arm above is the whole
 * difference — and telling a user with an imported audiobook library that
 * Audiobooks has *no catalogue source* would be false on the screen whose whole
 * job is "what do I have?". The truthful split ships and the conflict is
 * reported; §8.4 is not this file's to edit.
 *
 * ⚠️ COMICS IS THE SERIES, NEVER THE ISSUE. ADR-0068 decision 1 mints one
 * `comic_issue` per file UNDER a `comic` parent, and `comic_issue` is one of the
 * child kinds `store.recentWorkKinds` excludes (ADR-0030: *"a manga library's
 * chapters would swamp the number and Block A's row would report chapters where
 * the user reads series"*). So §17.2's *"comics are `series` in all three
 * places"* is what the code counts.
 */
const KIND_CATALOGUES: Record<string, readonly MediaType[]> = {
	kavita: ['ebooks', 'comics'],
	bookorbit: ['ebooks', 'audiobooks', 'comics']
};

/**
 * The media types SOME CONNECTED SERVICE can write works of, on this install.
 *
 * The precedent is `librarygrid`'s `browseEmptyState`, which already separates
 * "a library-bearing service is connected" from "this type has no rows" off the
 * same services read rather than off a constant. Same read, same separation,
 * one block earlier.
 *
 * Read off `kind` and not off `role`: `role` says a service HAS a catalogue,
 * and this question is which SIX-TYPE ROWS that catalogue can ever fill, which
 * only the adapter knows. A disabled instance still counts — its rows are in the
 * local file and the six counts include them — so nothing here reads `enabled`.
 */
export function catalogueReach(health: ServicesHealth | undefined): ReadonlySet<MediaType> {
	const reach = new Set<MediaType>();
	for (const service of health?.services ?? []) {
		for (const type of KIND_CATALOGUES[service.kind] ?? []) reach.add(type);
	}
	return reach;
}

/**
 * WHAT THE SIX COUNTS MEAN ON THIS INSTALL, which is a different question from
 * what they are.
 *
 * ⚠️ THE STATE THIS EXISTS FOR IS THE FIRST-RUN ONE, and it is the state §17.2
 * legislates by name: *"what Block A's columns mean while an import is running,
 * stated because that is where the first-run user spends their first several
 * minutes"*. The block used to render the moment the facet read landed, which is
 * before the first walk has written a row, so a user who had just connected a
 * service was told `Ebooks 0 books · Audiobooks 0 audiobooks · Comics 0 series`
 * for the several minutes the import ran. Every one of those zeros was a real
 * value off the wire and every one of them was a false sentence on the screen.
 *
 *   `not-counted`  no library-bearing instance has ever completed a full
 *                  import and none has contributed a work. Nothing has been
 *                  counted, so no row may render a number.
 *   `partial`      no completed import, but committed batches have landed
 *                  (`workCount > 0`). The numbers are real and are not totals —
 *                  the same call `services.ts` makes for its own `Partial
 *                  import` cell, where *"a partial import's committed rows are
 *                  real and are shown"*.
 *   `counted`      at least one library-bearing instance has completed a full
 *                  import (`lastFullSyncAt !== null`).
 *
 * The clock is `ServiceHealth.lastFullSyncAt` off `GET /api/v1/services/health`,
 * which Home already fetches for Block B, and it is a specified instant rather
 * than a plausible one: the run's START, never its finish (http-api.md §3.5).
 * Nothing here renders it — the question asked of it is only whether it is
 * `null` — so the "never render a banner timestamp off null" rule is not in
 * play.
 *
 * `isIndexer` is the same predicate `homeMode` takes, rather than a second copy
 * of §8.5's test: an indexer reports `null` / `0` exactly like an unsynced
 * catalogue source and only `role` separates them (http-api.md §3.2).
 */
export type CountBasis = 'not-counted' | 'partial' | 'counted';

export function countBasis(health: ServicesHealth | undefined): CountBasis {
	const bearing = (health?.services ?? []).filter((service) => !isIndexer(service));
	if (bearing.some((service) => service.lastFullSyncAt !== null)) return 'counted';
	if (bearing.some((service) => service.workCount > 0)) return 'partial';
	return 'not-counted';
}

/**
 * THE ONE SENTENCE ABOVE THE BLOCK (§17.2), AND IT IS NOT §17.2's SENTENCE.
 *
 * §17.2 requires a caption above Block A and writes it out: *"`Items` is the
 * source's declared total from first contact, and it says so once above the
 * block — 'Totals reported by each service.'"* **That literal string is refused
 * here, deliberately, and the requirement is not.** `internal/store/facets.go`
 * is `SELECT COUNT(*)` over LOCAL rows under the caller's access scope; it never
 * asks a service for a declared total and there is no field on the wire carrying
 * one. Shipping §17.2's words would put a source's number on screen that UsArr
 * has not got — DESIGN-DIRECTION §9.6's fabrication ban, expressed as a caption.
 * So the caption ships and says what this read can actually support, which is
 * also the thing §17.2 wanted a caption FOR: that a number under an unfinished
 * import is not a total.
 *
 * One sentence each, §9.6, and no em dash in copy short enough not to need one
 * (§13).
 */
export const SUMMARY_CAPTION: Record<CountBasis, string> = {
	'not-counted':
		'No import has finished yet, so nothing here has been counted. The first one starts on ' +
		'its own and runs for minutes rather than seconds on a large library.',
	partial:
		'An import is still running, so these are the rows that have landed so far and not ' +
		'totals.',
	counted: 'Counted in UsArr’s own catalogue, never a total reported by a service.'
};

/** The caption for this install, so the template holds no copy of the mapping. */
export function summaryCaption(health: ServicesHealth | undefined): string {
	return SUMMARY_CAPTION[countBasis(health)];
}

/**
 * WHAT WOULD FILL A ROW NOTHING CATALOGUES, AND THE UNIT NOUN FOR ONE THAT IS
 * COUNTED.
 *
 * §17.2 gives Block A six rows and two shapes for them: a type with a catalogue
 * source renders a COUNT, and a type without one renders §17.7's per-type
 * `unconfigured` state — *"the type, `no catalogue source connected`, the
 * service that will populate it and the milestone it arrives in, and a link to
 * Add"*. `design/DESIGN-DIRECTION.md` rule 13 is what makes the second shape
 * legal beside the "never render a section with no content" rule: those rows
 * carry a state, a cause and an action, which is not nothing.
 *
 * ⚠️ WHICH SHAPE A ROW TAKES IS `catalogueReach`'s ANSWER AND NOT THIS TABLE'S.
 * What lives here is what each row SAYS in either shape, which is fixed by the
 * build: the service a user would add to fill this type, the milestone it
 * arrives in, and the noun its count takes. `service` and `milestone` are read
 * only on the sourceless shape and `unit` only on the counted one.
 *
 * `unit: ''` IS THE MARK OF A TYPE NO ADAPTER IN THIS BUILD CAN WRITE, and
 * `KIND_CATALOGUES` is the other half of that same fact. A noun declared for a
 * type nothing counts is a noun nobody has had to make true yet — §17.2's own
 * `artists` for Music is already wrong against this read, which would sum
 * `artist` AND `album` works into one number — so the three rows no kind
 * reaches carry none, and a test pins the two halves against each other.
 *
 * WHAT IS ACTUALLY MEASURED, kind by kind. `store.recentWorkKinds` is the
 * counted allowlist — `movie`, `series`, `artist`, `album`, `book`, `comic` —
 * and NOTHING in `internal/libsync` writes the first four:
 *
 *   movies       no writer. Radarr, v0.2 (ADR-0045).
 *   tv           no writer. Sonarr, v0.2 (ADR-0045).
 *   music        no writer. Navidrome, §16.1 slot #1, after v0.1.
 *   ebooks       `book` works, minus the audiobook half.
 *   audiobooks   `book` works whose primary edition is `format = 'audiobook'`.
 *   comics       `comic` works, the series and never the issue.
 */
interface TypeFacts {
	/** The unit noun a count takes, or '' where no adapter writes this type. */
	unit: string;
	/** The service a user would add to fill this row. */
	service: string;
	/** The milestone that service arrives in. */
	milestone: string;
}

const TYPE_FACTS: Record<MediaType, TypeFacts> = {
	movies: { unit: '', service: 'Radarr', milestone: 'v0.2' },
	tv: { unit: '', service: 'Sonarr', milestone: 'v0.2' },
	music: { unit: '', service: 'Navidrome', milestone: 'after v0.1' },
	// The mockup's own noun for this row (`424 books`). §17.2 gives worked
	// examples for three of the six — `films`, `artists`, `series` — and leaves
	// the book pair to the screen. Either v0.1 adapter fills it, so the row names
	// both rather than picking one for a user who has neither.
	ebooks: { unit: 'books', service: 'BookOrbit or Kavita', milestone: 'v0.1' },
	// `audiobooks` and not `books`, even though both rows count `book` works.
	// The two counts ARE comparable, so §17.2's mixed-unit argument does not
	// force a distinct noun here; what does is that the Ebooks row already took
	// the general word, and one column reading `424 books` above `118 books`
	// invites the reading that the second is a subset of the first. It is not:
	// every book lands in exactly one of the two (ADR-0059). One service fills
	// this row, because Kavita serves no audio.
	audiobooks: { unit: 'audiobooks', service: 'BookOrbit', milestone: 'v0.1' },
	// §17.2, verbatim: "comics are `series` in all three places, not `comic
	// series` in one of them". ADR-0068 is why that is also what is counted.
	comics: { unit: 'series', service: 'BookOrbit or Kavita', milestone: 'v0.1' }
};

/** §17.7's words for a type nothing can fill. One sentence, §9.6. */
export const NO_CATALOGUE_SOURCE = 'no catalogue source connected';

/**
 * Grouped, en-GB, because six numbers in one column are read against each other
 * and `1204` beside `553` is a comparison the reader has to do twice.
 * `$lib/libraries` takes the same formatter for the same column on Libraries.
 */
const COUNT = new Intl.NumberFormat('en-GB');

/**
 * The three words the `Status` column may hold, and every row carries one.
 *
 * ⚠️ A `Status` COLUMN THAT IS BLANK ON A HEALTHY ROW READS AS *status
 * unknown*, NOT AS *status fine*, and that is what shipped: the cell rendered
 * content only on a sourceless row, so Ebooks, Audiobooks and Comics each had a
 * header promising a state above an empty box. §17.7 has an `ok` state and the
 * services read now says which rows are in it, so the column keeps its header
 * and delivers on it rather than being renamed down to what it managed.
 *
 * `Catalogued` is deliberately not `Up to date`. There is no periodic re-sync in
 * this build — `cmd/usarr/import.go` runs at most one import per instance per
 * database, on connect, with no timer behind it — so a freshness claim would be
 * one nothing measures. What the word asserts is exactly what was checked: an
 * import completed and these rows came out of it.
 */
export const SUMMARY_STATE = {
	/** No connected service can write works of this type. */
	unconfigured: NO_CATALOGUE_SOURCE,
	/** A connected service writes this type and no import has finished yet. */
	importing: 'first import running',
	/** §17.7's `ok`: an import completed and this number came out of it. */
	ok: 'catalogued'
} as const;

export type SummaryState = keyof typeof SUMMARY_STATE;

/** One row of Block A, in `MEDIA_TYPES` order. */
export interface SummaryRow {
	mediaType: MediaType;
	/** The Type cell. `$lib/library`'s label, never a second copy of it. */
	label: string;
	/**
	 * Whether a SERVICE CONNECTED TO THIS INSTALL writes works of this type.
	 *
	 * ⚠️ NOT "whether this build has an adapter for it", which is what it meant
	 * when a Kavita-only install was told `Audiobooks · 0 audiobooks`. See
	 * `catalogueReach`.
	 */
	catalogued: boolean;
	/** §17.7's state for this row, which every row has. */
	state: SummaryState;
	/** The word the Status cell renders. `SUMMARY_STATE[state]`, carried on the
	 *  row so the template holds no second copy of the mapping. */
	status: string;
	/**
	 * The Items cell, unit noun included — `1,204 films`, `553 series` — or ''
	 * wherever there is no number this screen may state.
	 *
	 * ⚠️ THE NOUN IS IN THE CELL AND NOT IN THE HEADER, which §17.2 makes a
	 * correctness rule rather than a layout preference: the six units are not
	 * comparable, so *"rendered bare, `Music 612` reads as a smaller library than
	 * `Movies 1,204` when it is 4,118 albums"*, and *"a mixed-unit column labels
	 * its unit or it is misinformation"*.
	 */
	items: string;
	/** The service that would catalogue this type, or '' where one already does. */
	service: string;
	/** The milestone it arrives in, or ''. */
	milestone: string;
}

/**
 * Block A's six rows, from the six counts and the services read.
 *
 * ⚠️ IT TAKES THE HEALTH RESPONSE BECAUSE BOTH OF ITS RULES ARE ABOUT THE
 * INSTALL AND NEITHER IS ABOUT THE BUILD. Which types have a source is
 * `catalogueReach`'s answer, and whether any number has been counted at all is
 * `countBasis`'s; a constant in place of either one told a first-run user and a
 * Kavita-only user the same false sentence from opposite directions. `undefined`
 * is the pre-read state and yields no reach and `not-counted`, which draws no
 * number anywhere — the honest reading of "the health response has not landed".
 *
 * ⚠️ A ZERO IS RENDERED AS A ZERO AND NEVER AS "HIDDEN". `internal/store/
 * facets.go` collapses a restricted scope onto the same `0` an empty type
 * produces, deliberately and in one direction only, because publishing
 * "restricted" as its own value *"would turn every hidden type into a positive
 * statement that content exists"*. So `0 books` on a counted row is the whole of
 * what this screen may say, and it is true either way: the caller can see no
 * books of that type.
 *
 * ⚠️ AND A ROW THAT HAS NOT BEEN COUNTED IS NOT GIVEN THAT ZERO. Its count is
 * `0` on the wire too, and the two facts are not the same fact — `0 books` under
 * a finished import is a measurement, and `0 books` while the walk is still
 * running is a guess that happens to be rendered in digits. The row says what is
 * actually the case: which service will fill it, or that the import has not
 * finished.
 *
 * ALL SIX ROWS ARE RETURNED. §17.2's *"a type the user does not have is not
 * shown at all"* is satisfied by the `unconfigured` state rather than by
 * omission (rule 13), and dropping the sourceless rows leaves a Home showing
 * only the types one source covers, *"from which the only available inference
 * is that UsArr does not do the others"*.
 */
export function librarySummary(
	counts: MediaTypeCounts,
	health: ServicesHealth | undefined
): SummaryRow[] {
	const reach = catalogueReach(health);
	const basis = countBasis(health);
	return MEDIA_TYPES.map((mediaType) => {
		const facts = TYPE_FACTS[mediaType];
		const catalogued = reach.has(mediaType);
		const state: SummaryState = !catalogued
			? 'unconfigured'
			: basis === 'counted'
				? 'ok'
				: 'importing';
		const row: SummaryRow = {
			mediaType,
			label: mediaTypeLabel(mediaType),
			catalogued,
			state,
			status: SUMMARY_STATE[state],
			items: '',
			service: '',
			milestone: ''
		};
		// A number only where one has been counted. `not-counted` is the first-run
		// window and `partial` is a running import whose committed batches are
		// real — services.ts makes the same call for its own `Partial import`
		// cell, where "a partial import's committed rows are real and are shown".
		if (catalogued && basis !== 'not-counted') {
			row.items = `${COUNT.format(counts[mediaType])} ${facts.unit}`;
		}
		if (!catalogued) {
			row.service = facts.service;
			row.milestone = facts.milestone;
		}
		return row;
	});
}

/**
 * The count beside the heading: `6 media types, 3 catalogued`.
 *
 * BOTH CLAUSES, because the first alone is true and misleading — half the rows
 * hold no number, and a reader who takes `6 media types` as the subject of the
 * table below has been told the wrong thing about it. It is derived from the
 * rows rather than written down, so it cannot disagree with them.
 *
 * ⚠️ THE SECOND CLAUSE MOVES WITH THE INSTALL, and that is `catalogueReach`'s
 * doing rather than a change here: a Kavita-only install reads `6 media types,
 * 2 catalogued` and a BookOrbit one reads `3`, because the row this counts is
 * the row that has a source connected and not the row the build could fill.
 */
export function summaryCount(rows: readonly SummaryRow[]): string {
	const catalogued = rows.filter((r) => r.catalogued).length;
	return `${rows.length} media types, ${catalogued} catalogued`;
}
