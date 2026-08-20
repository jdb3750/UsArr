import { describe, expect, it } from 'vitest';
// HOME'S TEMPLATE AS TEXT, for the copy guard at the bottom of this file. `?raw`
// is Vite's own mechanism and it types as `string`, which is what lets a ban
// list run over inline markup in an `environment: 'node'` vitest run with no
// Svelte plugin. See `$lib/copyguard` for why the stripping is not written here.
import HOME_SOURCE from '../routes/+page.svelte?raw';
import { sectionsMarkup, snippetMarkup, userFacingMarkup } from './copyguard';
import {
	attention,
	catalogueReach,
	countBasis,
	hasIndexer,
	headline,
	homeMode,
	HOME_SEARCH_SCOPE_NOTE,
	librarySummary,
	needsAttention,
	NO_CATALOGUE_SOURCE,
	summaryCaption,
	summaryCount,
	SUMMARY_CAPTION
} from './home';
import { MEDIA_TYPES, NO_MEDIA_TYPE_COUNTS, type MediaTypeCounts } from './library';
// §17.5's ban list, imported rather than restated: Home renders the same grab
// vocabulary the Requests block does, and two copies of a ban list is two lists.
import { FORBIDDEN_OUTCOME_WORDS } from './requests';
import { rollup, rollupCount, type ServiceRow } from './services';
import type { ServiceHealth, ServicesHealth } from './api';

/**
 * HOME's vocabulary, pinned. ARCHITECTURE.md §17.2 (as amended by ADR-0028),
 * §17.7 and §8.5.
 *
 * Every assertion here is a rule one of those states as a rule rather than as
 * taste, and the ones that are easiest to break silently are:
 *
 *  1. Search-and-Grab mode is DERIVED from the health response's `role`, which
 *     is §8.5's own test — "no configured instance advertises LibrarySync" —
 *     and never from a constant. A hard-coded mode goes on claiming there is
 *     no library on the first build that connects a Sonarr.
 *  2. Nothing configured is `unconfigured` and never an empty Home (§17.7).
 *  3. Block B is hidden when empty, so `attention()` must return nothing at
 *     all rather than a row that says everything is fine. A green "all good"
 *     panel is the thing the block must never become.
 *  4. Block B and the Services roll-up answer the same question and must agree
 *     on the count, which is why both go through `needsAttention`.
 *  5. The page head is derived too. The mockup records the failure: a constant
 *     "Last delta sync 14:02" sat above "No services configured" on the first
 *     screen a new user saw.
 */

const NOW = new Date('2026-08-16T14:08:00Z');

function health(over: Partial<ServiceHealth> = {}): ServiceHealth {
	return {
		id: 1,
		name: 'Prowlarr',
		kind: 'prowlarr',
		role: 'indexer',
		baseUrl: 'http://10.0.0.4:9696',
		enabled: true,
		state: 'healthy',
		breakerState: 'closed',
		consecutiveFailures: 0,
		warnings: [],
		blockedIndexers: [],
		stale: false,
		// The two catalogue-freshness fields are REQUIRED on ServiceHealth, so
		// they are supplied here even though Home reads neither: `attention()`
		// looks at state and breaker only. Home's block A counts and its block C
		// table have their own sources, and neither is `work_count` — see
		// itemsCell() in ./services for what that number is scoped to.
		lastFullSyncAt: null,
		workCount: 0,
		// Required too, and Home reads it no more than it reads the other two.
		fileReadFailures: 0,
		...over
	};
}

function payload(over: Partial<ServicesHealth> = {}): ServicesHealth {
	return { services: [], anyUnhealthy: false, setupRequired: false, ...over };
}

describe('homeMode', () => {
	it('is unconfigured when the server says setup is required', () => {
		expect(homeMode(payload({ setupRequired: true }))).toBe('unconfigured');
	});

	it('is unconfigured when nothing came back, even without the flag', () => {
		// A response with no services and no flag is still nothing configured, and
		// §17.7 sends that to the first-run path rather than to an empty Home.
		expect(homeMode(payload())).toBe('unconfigured');
	});

	it('is Search-and-Grab when every configured instance is an indexer', () => {
		expect(homeMode(payload({ services: [health()] }))).toBe('search-and-grab');
	});

	it('leaves Search-and-Grab as soon as one instance is library-bearing', () => {
		// §8.5's activation test is "no configured instance advertises
		// LibrarySync", so ONE library-bearing service is enough to leave the
		// mode. The role is what carries it; nothing here is hard-coded to
		// prowlarr.
		const rows = [health(), health({ id: 2, name: 'Radarr', kind: 'radarr', role: 'library' })];
		expect(homeMode(payload({ services: rows }))).toBe('library');
	});

	it('reads the role rather than the kind, so an unknown kind still classifies', () => {
		const rows = [health({ kind: 'komga', role: 'library' })];
		expect(homeMode(payload({ services: rows }))).toBe('library');
	});
});

describe('attention', () => {
	it('is empty when every service is healthy, so Block B is hidden', () => {
		// §17.2: hidden ENTIRELY when empty, never a green "all good" panel. The
		// block's {#if} keys on this being empty, so a row describing health here
		// would put the panel on the screen.
		expect(attention([health(), health({ id: 2, name: 'Prowlarr 4K' })], NOW)).toEqual([]);
	});

	it('carries UsArr’s own word and the upstream’s verbatim text separately', () => {
		const rows = attention(
			[health({ state: 'down', consecutiveFailures: 3, problem: '401 Unauthorized\nbody: {}' })],
			NOW
		);
		expect(rows).toHaveLength(1);
		// State is UsArr speaking. The mechanism's vocabulary — `down`, `breaker
		// open` — belongs behind the Services row expander and not here.
		expect(rows[0].state).toBe('not answering');
		expect(rows[0].detail).toBe('3 failed attempts');
		// Problem is the upstream's own words, whole and untouched. The screen
		// truncates for the cell; the row carries the full string.
		expect(rows[0].problem).toBe('401 Unauthorized\nbody: {}');
		expect(rows[0].tone).toBe('err');
	});

	it('reports a service that has never been probed rather than assuming it is fine', () => {
		const rows = attention([health({ stale: true })], NOW);
		expect(rows).toHaveLength(1);
		expect(rows[0].state).toBe('not checked yet');
		// `none`, not `ok`: nothing was observed, and a green tick would claim a
		// measurement UsArr has not taken.
		expect(rows[0].tone).toBe('none');
	});

	it('reports a service the owner turned off', () => {
		const rows = attention([health({ enabled: false })], NOW);
		expect(rows.map((r) => r.state)).toEqual(['turned off']);
	});

	it('has no problem text when the upstream said nothing', () => {
		expect(attention([health({ enabled: false })], NOW)[0].problem).toBe('');
	});
});

describe('Block B and the Services roll-up agree', () => {
	// Two lists answering "what is wrong" from two predicates is how one screen
	// reports three problems while the other reports four, and the only reading
	// available to the user is that one of them is lying. Both go through
	// `needsAttention`, and this is the test that keeps them there.
	const services = [
		health(),
		health({ id: 2, name: 'Down', state: 'down', consecutiveFailures: 2 }),
		health({ id: 3, name: 'Off', enabled: false }),
		health({ id: 4, name: 'Unprobed', stale: true }),
		health({ id: 5, name: 'Wobbly', state: 'degraded', consecutiveFailures: 1 })
	];
	const rows: ServiceRow[] = services.map((h) => ({ health: h }));

	it('selects the same instances', () => {
		expect(attention(services, NOW).map((r) => r.id)).toEqual(rollup(rows, NOW).map((r) => r.id));
	});

	it('counts them the same way', () => {
		expect(rollupCount(attention(services, NOW))).toBe(rollupCount(rollup(rows, NOW)));
	});

	it('keeps a healthy row out of both', () => {
		expect(needsAttention({ health: health() }, NOW)).toBe(false);
	});
});

describe('headline', () => {
	it('says it is still reading before the response lands', () => {
		expect(headline(undefined, 0, '')).toBe('Reading what is connected.');
	});

	it('never reports a sync over a system with nothing connected', () => {
		// The failure the mockup records, as a test: a constant "Last delta sync
		// 14:02, 6 minutes ago" above "No services configured".
		const line = headline('unconfigured', 0, '');
		expect(line).toBe('No service is connected yet.');
		expect(line).not.toMatch(/sync/i);
	});

	it('names the mode and the count', () => {
		expect(headline('search-and-grab', 2, '1 error, 1 warning')).toBe(
			'Search-and-Grab mode. 2 services connected. 1 error, 1 warning.'
		);
	});

	it('singularises one service', () => {
		expect(headline('search-and-grab', 1, '')).toBe(
			'Search-and-Grab mode. 1 service connected. Nothing needs attention.'
		);
	});

	it('claims nothing needs attention rather than that every service is healthy', () => {
		// The roll-up skips an enabled instance whose probe ran and could not
		// classify it, so an empty roll-up does not establish that everything is
		// healthy. It establishes exactly what the shorter claim says.
		const line = headline('search-and-grab', 3, '');
		expect(line).toContain('Nothing needs attention.');
		expect(line).not.toMatch(/healthy/i);
	});

	it('does not claim Search-and-Grab once a library-bearing service exists', () => {
		expect(headline('library', 1, '')).not.toMatch(/Search-and-Grab/);
	});
});

/**
 * THE SEARCH ENTRY POINT'S TWO RULES, and both are rules rather than taste.
 *
 *  1. A box is drawn only where something can answer it. `hasIndexer` is the
 *     precondition, NOT the mode: an install with a Sonarr and a Prowlarr is in
 *     `library` mode and has a perfectly good indexer, and gating on the mode
 *     would delete a working control the day a Sonarr is accepted.
 *  2. The box says what it searches. DESIGN-DIRECTION §8.3 keeps library search
 *     and release search apart — merging them is how a 0 ms local query waits on
 *     a 30 s indexer — so an unlabelled input, which reads as library search by
 *     default, is the failure.
 */
describe('hasIndexer', () => {
	it('is false on a fresh install, so Home never draws a box with nothing behind it', () => {
		expect(hasIndexer(payload())).toBe(false);
	});

	it('is true whenever an indexer is configured', () => {
		expect(hasIndexer(payload({ services: [health()] }))).toBe(true);
	});

	it('stays true in library mode, because an indexer beside a Sonarr still answers', () => {
		const rows = [health(), health({ id: 2, name: 'Radarr', kind: 'radarr', role: 'library' })];
		const health_ = payload({ services: rows });
		expect(homeMode(health_)).toBe('library');
		expect(hasIndexer(health_)).toBe(true);
	});

	it('is false when the only service is library-bearing', () => {
		const rows = [health({ kind: 'radarr', role: 'library' })];
		expect(hasIndexer(payload({ services: rows }))).toBe(false);
	});
});

/**
 * ⚠️ THESE TWO ASSERTIONS ARE THE INVERSE OF WHAT THEY USED TO BE, AND THEY WERE
 * INVERTED RATHER THAN RELAXED. They read `toMatch(/indexers/i)`,
 * `toMatch(/Requests/)` and `toMatch(/not your own library/i)` against a note
 * that said *"This searches releases on your indexers, not your own library."*
 * The owner moved Home's box onto the local corpus, so every one of those
 * assertions now pins the string to a claim the product does not make. The
 * SHAPE of the guard is what survives and is what mattered: the note must name
 * a corpus and must name the corpus it is NOT, because a box labelled only
 * `Search` is the §8.3 merge arrived at by omission, and which side of the merge
 * the box sits on does not change that.
 */
describe('HOME_SEARCH_SCOPE_NOTE', () => {
	it('names the corpus it searches', () => {
		expect(HOME_SEARCH_SCOPE_NOTE).toMatch(/library/i);
		expect(HOME_SEARCH_SCOPE_NOTE).toMatch(/your services/i);
	});

	it('says explicitly that it is not your indexers', () => {
		expect(HOME_SEARCH_SCOPE_NOTE).toMatch(/not your indexers/i);
	});

	it('does not send the user to Requests, which §17.4 rule 6 already does later', () => {
		// A third mention of release search on the screen the owner has just said
		// carries too many searches. `routes/search` carries the exit instead.
		expect(HOME_SEARCH_SCOPE_NOTE).not.toMatch(/Requests/);
	});

	it('enumerates no media type, on §17.4 rule 7 reasoning', () => {
		// The corpus is whatever the connected services supply, so any fixed list
		// is a promise this install may not keep.
		for (const type of ['film', 'movie', 'episode', 'album', 'book', 'comic'])
			expect(HOME_SEARCH_SCOPE_NOTE.toLowerCase()).not.toContain(type);
	});
});

/* ── §17.5's ban, over Home's own Recent-grabs chrome ─────────────────────── */

/**
 * THE HOLE THIS CLOSES, STATED FIRST, BECAUSE IT WAS REAL AND NOT HYPOTHETICAL.
 *
 * §17.5's banned vocabulary is checked against shipped strings in three places
 * and, until this block, all three were on the Requests side of the app:
 * `requests.test.ts` holds the strings `requests.ts` exports, the markup of
 * `routes/requests/+page.svelte`, and the markup of `$lib/RecentGrabs.svelte`.
 *
 * Home draws grab copy too, and NONE of it was any of those three. The rows
 * themselves are the shared component and were covered the day it was extracted
 * — but the CHROME around them is Home's own: the `Recent grabs` heading, the
 * count beside it (`3 grabs`, or `the 10 most recent`), the show-all link, and
 * the whole unreadable-list banner, which is four sentences about what a
 * missing list does and does not mean. Every one of those is a statement about
 * a grab, written inline in `routes/+page.svelte`, and no guard read the file.
 * A `failed` typed into that banner would have shipped green.
 *
 * ⚠️ THE REGION IS BOTH ARMS OF THE `{#if}`, WHICH IS WHY THE SLICER RETURNS A
 * LIST. Home renders `id="home-grabs"` twice — once for the error banner and
 * once for the loaded table — and a guard that took the first match would have
 * read the banner and silently dropped the heading, the count and the link.
 * `sectionsMarkup` throws rather than returning nothing when the marker moves,
 * for the reason the Requests guard records: an empty corpus passes every
 * `not.toContain` there is.
 *
 * WHAT THIS BLOCK DELIBERATELY DOES NOT DO. It does not re-check the rows: they
 * are `$lib/RecentGrabs.svelte`, which `requests.test.ts` already holds against
 * the same list, and a second copy of that assertion would be the duplication
 * the component was extracted to remove. And it does not extend the subset ban
 * across the REST of Home — the search entry point and the three mode blocks —
 * which is a real gap and a separate change, because those are statements about
 * a search and about configuration rather than about a download.
 */
const HOME_GRAB_MARKUP = sectionsMarkup(userFacingMarkup(HOME_SOURCE), 'id="home-grabs"').join(
	'\n'
);

describe('the banned vocabulary, in Home’s Recent-grabs chrome', () => {
	it('is reading the region it thinks it is reading', () => {
		// Both arms, each pinned to a string only that arm carries. A guard that
		// matches nothing is indistinguishable from no guard, and after the
		// `{#if}` there are two ways to match half of it rather than one.
		expect(HOME_GRAB_MARKUP).toContain('Recent grabs');
		expect(HOME_GRAB_MARKUP).toContain('Your recent grabs could not be read');
		expect(HOME_GRAB_MARKUP).toContain('most recent');
		expect(HOME_GRAB_MARKUP).toContain('All recent grabs on Requests');
	});

	it.each(FORBIDDEN_OUTCOME_WORDS)('never says “%s” about a grab', (word) => {
		expect(HOME_GRAB_MARKUP.toLowerCase()).not.toContain(word);
	});

	it('never asserts in the markup that a grab did not happen', () => {
		// The same two phrases the Requests guard pins, and the reason Home needs
		// them is that its banner is the one place in the app that comes closest:
		// "Nothing is missing here because a grab did not happen" is the DENIAL of
		// the claim, and an edit that dropped its first four words would turn it
		// into the assertion §17.5 forbids.
		expect(HOME_GRAB_MARKUP.toLowerCase()).not.toContain('did not go through');
		expect(HOME_GRAB_MARKUP.toLowerCase()).not.toContain('nothing was sent');
	});

	it('keeps saying that an unreadable list is not an empty one', () => {
		// The counter-case, pinned so a later tightening of the ban has to face
		// it. This sentence is the point of the banner: a local read failing says
		// something about UsArr, not about whether anything was ever grabbed.
		expect(HOME_GRAB_MARKUP).toContain('a grab did not happen');
	});
});

/* =============================================================================
 * BLOCK A — the media-type summary (§17.2, ADR-0028)
 * ========================================================================== */

/** Counts as `GET /api/v1/library/facets` answers them on a real BookOrbit. */
function counts(over: Partial<MediaTypeCounts> = {}): MediaTypeCounts {
	return { ...NO_MEDIA_TYPE_COUNTS, ebooks: 424, audiobooks: 118, comics: 553, ...over };
}

/**
 * A library-bearing instance that has COMPLETED a full import, which is the
 * state every count assertion below needs and the state a first-run install is
 * not in. The timestamp is read for `null` versus not-null and never rendered,
 * so its value only has to be a string.
 */
function imported(over: Partial<ServiceHealth> = {}): ServiceHealth {
	return health({
		id: 2,
		name: 'BookOrbit',
		kind: 'bookorbit',
		role: 'library',
		lastFullSyncAt: '2026-08-16T13:02:00Z',
		workCount: 1095,
		...over
	});
}

/** The whole health payload for an install with one imported BookOrbit. */
function bookorbit(over: Partial<ServiceHealth> = {}): ServicesHealth {
	return payload({ services: [imported(over)] });
}

describe('Block A’s rows', () => {
	it('draws all six types, in the media-type enum’s own order', () => {
		// §17.2 rejects dropping the sourceless rows explicitly: a Home showing
		// only the types one source covers leaves "the only available inference …
		// that UsArr does not do the others". The order is MEDIA_TYPES', not a
		// second list that could drift from the sidebar's.
		const rows = librarySummary(counts(), bookorbit());
		expect(rows.map((r) => r.mediaType)).toEqual([...MEDIA_TYPES]);
		expect(rows.map((r) => r.label)).toEqual([
			'Movies',
			'TV',
			'Music',
			'Ebooks',
			'Audiobooks',
			'Comics'
		]);
	});

	it('names its unit in every cell that carries a number', () => {
		// §17.2 makes this a correctness rule, not a layout preference: "a
		// mixed-unit column labels its unit or it is misinformation", because
		// rendered bare "Music 612 reads as a smaller library than Movies 1,204
		// when it is 4,118 albums".
		const rows = librarySummary(counts(), bookorbit());
		const items = Object.fromEntries(rows.map((r) => [r.mediaType, r.items]));
		expect(items.ebooks).toBe('424 books');
		expect(items.audiobooks).toBe('118 audiobooks');
		// §17.2, verbatim: "comics are `series` in all three places". It is also
		// what is counted — ADR-0068 mints a `comic_issue` under a `comic` parent,
		// and `recentWorkKinds` excludes the child kind, so the number is series.
		expect(items.comics).toBe('553 series');
	});

	it('groups a large count rather than printing it bare', () => {
		const rows = librarySummary(counts({ comics: 1204, ebooks: 28904 }), bookorbit());
		const items = Object.fromEntries(rows.map((r) => [r.mediaType, r.items]));
		expect(items.comics).toBe('1,204 series');
		expect(items.ebooks).toBe('28,904 books');
	});

	it('gives every sourceless row a service and a milestone, and no number', () => {
		// §17.2's per-type `unconfigured` state (`docs/ARCHITECTURE.md`),
		// which is what makes these rows legal beside §17.1's "never render a
		// section with no content" rule (DESIGN-DIRECTION rule 13): a state, a
		// cause and an action.
		//
		// ⚠️ CITED §17.7 UNTIL 2026-08-20. §17.7 does not contain the word
		// `unconfigured` anywhere; it specifies install-level screens, and the
		// install-level §17.7 citations in this file's header are SOUND and were
		// left alone. §17.2 writes this state `(§17.7)` itself, so the
		// mis-citation originates in the design document and `web/` copied it
		// faithfully.
		const rows = librarySummary(counts(), bookorbit());
		const sourceless = rows.filter((r) => !r.catalogued);
		expect(sourceless.map((r) => [r.mediaType, r.service, r.milestone])).toEqual([
			['movies', 'Radarr', 'v0.2'],
			['tv', 'Sonarr', 'v0.2'],
			['music', 'Navidrome', 'after v0.1']
		]);
		// No number on any of them. Their count is zero on the wire like every
		// other uncatalogued type, and "0 films" on an install where nothing has
		// ever looked for a film is a claim about the library, not the pipeline.
		expect(sourceless.map((r) => r.items)).toEqual(['', '', '']);
	});

	it('counts audiobooks as catalogued on a BookOrbit, because that adapter writes them', () => {
		/* THE ONE ROW A DOCUMENT AND THE CODE DISAGREE ABOUT.
		   `design/DESIGN-DIRECTION.md` §8.4 lists audiobooks among v0.1's
		   sourceless types, on the premise that "BookOrbit's media types are
		   Kavita's". They are not: Kavita serves no audio and its adapter says so
		   (`internal/libsync/kavita.go`, the LibraryTypeBook arm), while
		   BookOrbit's `bookOrbitEditionFormat` maps m4b/mp3/m4a/opus/ogg/flac onto
		   `edition.format = 'audiobook'` and calls that "the line the Audiobooks
		   screen depends on". Telling a user with an imported audiobook library
		   that Audiobooks has no catalogue source is false on the screen whose
		   whole job is "what do I have?". */
		const rows = librarySummary(counts(), bookorbit());
		const audiobooks = rows.find((r) => r.mediaType === 'audiobooks');
		expect(audiobooks?.catalogued).toBe(true);
		expect(audiobooks?.service).toBe('');
		expect(audiobooks?.milestone).toBe('');
	});

	it('renders an empty catalogue as a zero and never as “hidden”', () => {
		// `internal/store/facets.go` collapses a restricted scope onto the same
		// zero an empty type gives, deliberately and in one direction only:
		// "restriction renders as zero, and zero never widens into hidden or
		// unknown". Publishing "restricted" would be the existence oracle with a
		// label on it, so this side may not reintroduce it.
		const rows = librarySummary(NO_MEDIA_TYPE_COUNTS, bookorbit());
		const items = rows.filter((r) => r.catalogued).map((r) => r.items);
		expect(items).toEqual(['0 books', '0 audiobooks', '0 series']);
		for (const row of rows) {
			const said = `${row.items} ${row.service} ${row.milestone} ${row.status}`.toLowerCase();
			expect(said).not.toContain('hidden');
			expect(said).not.toContain('unknown');
			expect(said).not.toContain('restricted');
		}
	});

	it('renders a restricted scope with the SAME strings an empty library gets', () => {
		/* ⚠️ THIS TEST USED TO COMPARE `librarySummary(NO_MEDIA_TYPE_COUNTS)`
		   AGAINST `librarySummary({six literal zeros})` — two deep-equal literals
		   — SO IT ASSERTED ONLY THAT THE FUNCTION IS DETERMINISTIC. Proved by
		   mutation: reversing the row order broke three tests in this file and
		   left that one green, because a reversal applies equally to both sides.

		   What the collapse actually requires is a claim about the RENDERED
		   STRINGS, so it is those that are pinned. `internal/store/facets.go`
		   answers a restricted scope with the same six zeros an empty library
		   gives, in one direction only, and a screen that could tell the two apart
		   would be the existence oracle the collapse exists to remove. */
		const restricted = librarySummary(
			{ movies: 0, tv: 0, music: 0, ebooks: 0, audiobooks: 0, comics: 0 },
			bookorbit()
		);
		expect(restricted.map((r) => [r.label, r.items, r.status, r.service, r.milestone])).toEqual([
			['Movies', '', 'no catalogue source connected', 'Radarr', 'v0.2'],
			['TV', '', 'no catalogue source connected', 'Sonarr', 'v0.2'],
			['Music', '', 'no catalogue source connected', 'Navidrome', 'after v0.1'],
			['Ebooks', '0 books', 'catalogued', '', ''],
			['Audiobooks', '0 audiobooks', 'catalogued', '', ''],
			['Comics', '0 series', 'catalogued', '', '']
		]);
	});

	it('says how many of the six have a source, beside how many there are', () => {
		// "6 media types" alone is true and misleading while half of them hold no
		// number. Derived from the rows, so the head cannot disagree with them.
		expect(summaryCount(librarySummary(counts(), bookorbit()))).toBe('6 media types, 3 catalogued');
		expect(summaryCount([])).toBe('0 media types, 0 catalogued');
	});

	it('states the sourceless case in one sentence and without an em dash', () => {
		// §9.6: one sentence, no container, no centring. §13: no em dash in copy
		// short enough not to need one.
		expect(NO_CATALOGUE_SOURCE).toBe('no catalogue source connected');
		expect(NO_CATALOGUE_SOURCE).not.toContain('—');
	});
});

/**
 * WHICH ROWS HAVE A SOURCE IS A PROPERTY OF THE INSTALL, NOT OF THE BUILD.
 *
 * The defect these pin: `catalogued` was a static record, so a Kavita-only
 * install was told `Audiobooks · 0 audiobooks` — the exact claim about the
 * library, rather than about the pipeline, that the sourceless rows exist to
 * avoid. Kavita serves no audio (`internal/libsync/kavita.go`, LibraryTypeBook).
 */
describe('Block A’s reach, per install', () => {
	it('gives a Kavita install ebooks and comics, and no audiobooks at all', () => {
		const kavita = payload({
			services: [imported({ name: 'Kavita', kind: 'kavita', workCount: 553 })]
		});
		expect([...catalogueReach(kavita)].sort()).toEqual(['comics', 'ebooks']);
		const rows = librarySummary(counts(), kavita);
		const audiobooks = rows.find((r) => r.mediaType === 'audiobooks');
		// The row the static table got wrong, and the whole of the fix: no number,
		// the service that would fill it, and the state that says why.
		expect(audiobooks?.catalogued).toBe(false);
		expect(audiobooks?.items).toBe('');
		expect(audiobooks?.status).toBe('no catalogue source connected');
		expect(audiobooks?.service).toBe('BookOrbit');
		expect(summaryCount(rows)).toBe('6 media types, 2 catalogued');
	});

	it('gives a BookOrbit install the audio half as well', () => {
		expect([...catalogueReach(bookorbit())].sort()).toEqual(['audiobooks', 'comics', 'ebooks']);
	});

	it('unions the two when both are connected', () => {
		const both = payload({
			services: [imported(), imported({ id: 3, name: 'Kavita', kind: 'kavita' })]
		});
		expect([...catalogueReach(both)].sort()).toEqual(['audiobooks', 'comics', 'ebooks']);
	});

	it('gives a Prowlarr-only install no reach at all', () => {
		// `search-and-grab` mode, where Block A is not drawn anyway. The function
		// still has to answer honestly rather than fall back to the build's list.
		expect([...catalogueReach(payload({ services: [health()] }))]).toEqual([]);
		expect([...catalogueReach(undefined)]).toEqual([]);
	});

	it('reads the kind and not the role, so an unadapted library kind reaches nothing', () => {
		// `internal/httpapi.serviceKinds` may register a kind as `library` before
		// `cmd/usarr/import.go` has an adapter for it; `role` would then promise
		// rows nothing can write. A kind with no entry gets no types.
		const komga = payload({ services: [imported({ kind: 'komga' })] });
		expect([...catalogueReach(komga)]).toEqual([]);
	});

	it('counts a disabled instance, because its rows are already in the local file', () => {
		expect([...catalogueReach(bookorbit({ enabled: false }))].sort()).toEqual([
			'audiobooks',
			'comics',
			'ebooks'
		]);
	});

	it('declares a unit noun for exactly the types some adapter can write', () => {
		/* The two halves of one fact, pinned against each other. `unit: ''` marks a
		   type no adapter in this build writes, and `KIND_CATALOGUES` is what
		   decides which those are; a noun declared for a type nothing counts is a
		   noun nobody has had to make true yet, and §17.2's own `artists` for Music
		   is already wrong against this read. */
		const everyKind = payload({
			services: [imported(), imported({ id: 3, name: 'Kavita', kind: 'kavita' })]
		});
		const reachable = catalogueReach(everyKind);
		const rows = librarySummary(counts(), everyKind);
		for (const row of rows) {
			expect(row.catalogued).toBe(reachable.has(row.mediaType));
			// A reachable row has a noun after its digits; an unreachable one has no
			// cell at all rather than a bare number.
			if (row.catalogued) expect(row.items).toMatch(/^[\d,]+ [a-z]+$/);
			else expect(row.items).toBe('');
		}
	});
});

/**
 * WHAT THE SIX COUNTS MEAN WHILE THE FIRST IMPORT IS RUNNING.
 *
 * The defect these pin: the block rendered as soon as the facet read landed,
 * which is before the first walk had written a row, so a user who had just
 * connected a service was told `Ebooks 0 books · Audiobooks 0 audiobooks ·
 * Comics 0 series` for the several minutes it ran. §17.2 legislates this exact
 * moment and requires a caption above the block.
 */
describe('Block A while the first import runs', () => {
	/** The install a first-run user is on: connected, walking, nothing written. */
	const firstRun = payload({ services: [imported({ lastFullSyncAt: null, workCount: 0 })] });

	it('draws no number at all before anything has been counted', () => {
		const rows = librarySummary(NO_MEDIA_TYPE_COUNTS, firstRun);
		expect(rows.map((r) => r.items)).toEqual(['', '', '', '', '', '']);
		// And the three that DO have a source say which state they are in, so the
		// absence is explained on the row rather than left as a blank cell.
		expect(rows.filter((r) => r.catalogued).map((r) => r.status)).toEqual([
			'first import running',
			'first import running',
			'first import running'
		]);
	});

	it('shows the committed batches of a partial import, and says they are partial', () => {
		// services.ts makes the same call for its own `Partial import` cell: "a
		// partial import's committed rows are real and are shown". What may not be
		// claimed is that they are totals, and the caption is where that is said.
		const partial = payload({ services: [imported({ lastFullSyncAt: null, workCount: 300 })] });
		expect(countBasis(partial)).toBe('partial');
		const rows = librarySummary(counts(), partial);
		expect(rows.map((r) => r.items)).toEqual([
			'',
			'',
			'',
			'424 books',
			'118 audiobooks',
			'553 series'
		]);
		expect(rows[3].status).toBe('first import running');
	});

	it('calls a completed import counted, and only then', () => {
		expect(countBasis(bookorbit())).toBe('counted');
		expect(countBasis(firstRun)).toBe('not-counted');
		expect(countBasis(undefined)).toBe('not-counted');
		// An indexer reports null/0 exactly like an unsynced catalogue source, so
		// reading the two fields without `role` would leave a Prowlarr-only install
		// permanently "not counted" and, worse, would let one count for a library.
		expect(countBasis(payload({ services: [health()] }))).toBe('not-counted');
	});

	it('gives a counted row the ok state and never leaves Status blank', () => {
		// A column headed `Status` that is empty on a healthy row reads as *status
		// unknown*, not as *status fine*. Every row carries a word.
		//
		// ⚠️ THIS CASE WAS NAMED *"gives a counted row §17.7’s ok state…"* UNTIL
		// 2026-08-20, and the citation in that name was false: searching
		// `docs/ARCHITECTURE.md` and `docs/design/DESIGN-DIRECTION.md` found no
		// design source for a per-type `ok` state — that two-document search is
		// the whole boundary of the claim, and it does NOT say the state is
		// invented, unspecified or wrong. See `$lib/home`'s `SUMMARY_STATE` note.
		// The name was changed rather than annotated because a test name is
		// shipped descriptive text, and a false design citation inside quotation
		// marks is no more true than one after a `//`. The two states this case
		// also asserts, `unconfigured` and `importing`, ARE specified, both in
		// §17.2 of `docs/ARCHITECTURE.md`.
		const rows = librarySummary(counts(), bookorbit());
		expect(rows.map((r) => r.status)).toEqual([
			'no catalogue source connected',
			'no catalogue source connected',
			'no catalogue source connected',
			'catalogued',
			'catalogued',
			'catalogued'
		]);
		for (const row of rows) expect(row.status).not.toBe('');
	});

	it('captions the block with what the read supports, and never §17.2’s own string', () => {
		/* §17.2 writes the caption out — "Totals reported by each service." — and
		   that literal string is refused: `internal/store/facets.go` is
		   `SELECT COUNT(*)` over local rows and never asks a service for a declared
		   total, so shipping those words would put a provenance on screen UsArr has
		   not got (§9.6). The requirement is kept; the sentence is one this read
		   can support. */
		for (const caption of Object.values(SUMMARY_CAPTION)) {
			expect(caption).not.toContain('Totals reported by each service');
			expect(caption).not.toContain('—');
			expect(caption.length).toBeGreaterThan(20);
		}
		expect(summaryCaption(firstRun)).toContain('No import has finished yet');
		expect(summaryCaption(bookorbit())).toContain('Counted in UsArr’s own catalogue');
		expect(summaryCaption(payload({ services: [imported({ lastFullSyncAt: null })] }))).toContain(
			'still running'
		);
	});
});

/**
 * BLOCK A's CHROME, over the markup that actually ships.
 *
 * The ROWS are not read here and cannot be: every value in them is interpolated
 * from `librarySummary` above, so the template carries no label, no noun and no
 * service name to grep for. What IS written inline is the region's own copy —
 * the heading, the count beside it, the list's accessible label and the whole
 * uncountable-library banner — and none of that was covered by any guard.
 */
/**
 * ⚠️ THE SECTION AND THE SNIPPET THAT DRAWS ITS ROWS, AND THE SECOND HALF WAS
 * MISSING. This read `sectionsMarkup(…, 'id="home-summary"')` alone, which
 * slices to the first `</section>` — and every row of Block A is rendered by
 * `{#snippet summaryCellRender}`, which Svelte requires at the top level of the
 * component and which therefore lies outside every section on the page. Proved
 * by injection: `restricted · hidden · skeleton shimmer placeholder` was put
 * into the rendered sub-line of every sourceless row and all 99 tests passed.
 *
 * Both halves stripped by the same `userFacingMarkup` pass, so the guards below
 * cannot disagree with each other about what a reader can see.
 */
const HOME_SUMMARY_STRIPPED = userFacingMarkup(HOME_SOURCE);
const HOME_SUMMARY_MARKUP = [
	...sectionsMarkup(HOME_SUMMARY_STRIPPED, 'id="home-summary"'),
	snippetMarkup(HOME_SUMMARY_STRIPPED, 'summaryCellRender')
].join('\n');

describe('Block A’s chrome, in the markup', () => {
	it('is reading the region it thinks it is reading, snippet included', () => {
		// An empty string passes every not.toContain there is, so the positive
		// assertions come first and each names something only this section has.
		expect(HOME_SUMMARY_MARKUP.length).toBeGreaterThan(200);
		expect(HOME_SUMMARY_MARKUP).toContain('Your library');
		expect(HOME_SUMMARY_MARKUP).toContain('Your library could not be counted');
		expect(HOME_SUMMARY_MARKUP).toContain('Library by media type');
		// ⚠️ AND THE SNIPPET HALF, NAMED SEPARATELY. The three assertions above all
		// live inside the `<section>`, so every one of them passed while the rows'
		// own markup was outside the corpus. `Add` is the sourceless row's link and
		// exists nowhere else on this screen.
		expect(HOME_SUMMARY_MARKUP).toContain('>Add</a>');
		expect(HOME_SUMMARY_MARKUP).toContain('cell-sub');
	});

	it('declares exactly the three columns the wire can answer', () => {
		/* The column set is a `<script>` constant, which `userFacingMarkup` strips
		   — so this reads the RAW source, the way libraryscreen.test.ts reads
		   Home's `emptyTitle`. There is no other position to read it from: the
		   headers never appear in the template.

		   §17.2's row is `name · count · availability rollup · last import · see
		   all`, and `internal/httpapi/facets.go` answers the first two: the rollup
		   and the import time "are their own aggregates and their own commit". A
		   `Have` or `Synced` column would promise a figure this screen has not
		   computed, which is §9.6's fabrication ban applied to a column instead of
		   to a row. And `Have` is the one word the third column may not take,
		   because that is the availability rollup's own name (§9.5's
		   `have / total · N missing`). */
		const literal = /const SUMMARY_COLUMNS: ListColumn\[\] = \[([\s\S]*?)\n\t\];/.exec(HOME_SOURCE);
		expect(literal, 'SUMMARY_COLUMNS is not in routes/+page.svelte any more').not.toBeNull();
		const headers = [...(literal as RegExpExecArray)[1].matchAll(/header: '([^']+)'/g)].map(
			(m) => m[1]
		);
		expect(headers).toEqual(['Type', 'Items', 'Status']);
	});

	it('keeps exactly one column on the phone fork’s second line', () => {
		/* ⚠️ A TRIPWIRE FOR A CSS OVERRIDE, NOT A PREFERENCE ABOUT COLUMNS.
		   §17.2 wants "name and count on line one" and `.tbl--2line` gives every
		   line-1 cell `display: block`, so a second line-1 column starts its own
		   line — `424 books` rendered as a second title under `Ebooks`, measured at
		   390 px. `routes/+page.svelte`'s `<style>` block fixes it by making the
		   line-1 cells inline and BLOCKIFYING line 2 to end the line, scoped to
		   `#home-summary`.

		   That blockification is only correct while line 2 has exactly ONE cell.
		   `List.svelte` emits `.stacksep`'s inline `·` before every second-line
		   cell but the first, so a second one would carry its separator onto a
		   third line. §17.2 wants this block to grow `Have` and `Synced`, which is
		   precisely the change that would break it, so the constraint is asserted
		   here instead of trusted to a comment. If this fails: either keep the new
		   column on line 1, or rework the override in `+page.svelte` — and
		   re-measure at 390 px either way. */
		const literal = /const SUMMARY_COLUMNS: ListColumn\[\] = \[([\s\S]*?)\n\t\];/.exec(HOME_SOURCE);
		expect(literal, 'SUMMARY_COLUMNS is not in routes/+page.svelte any more').not.toBeNull();
		const secondLine = [...(literal as RegExpExecArray)[1].matchAll(/stackLine: 2/g)];
		expect(secondLine).toHaveLength(1);
		// And the override itself, so a rename of the section id cannot silently
		// unscope it or delete it.
		expect(HOME_SOURCE).toContain("#home-summary :global(.tbl--2line td[data-line='2'])");
		expect(HOME_SOURCE).toContain("#home-summary :global(.tbl--2line td[data-line='1'])");
	});

	it('never explains a zero as something being hidden from the caller', () => {
		const said = HOME_SUMMARY_MARKUP.toLowerCase();
		expect(said).not.toContain('hidden');
		expect(said).not.toContain('restricted');
		expect(said).not.toContain('not permitted');
	});

	it('keeps saying that an uncountable library is not an empty one', () => {
		// The counter-case, pinned so a later tightening has to face it. A local
		// read failing says something about UsArr and nothing about how much the
		// user has, and the banner is the one place that could imply otherwise.
		expect(HOME_SUMMARY_MARKUP).toContain('says nothing about how much you have');
	});

	it('draws no skeleton, no shimmer and no placeholder row', () => {
		// Six zeros is a real answer this endpoint gives, so a zeroed placeholder
		// is not a placeholder, it is a claim. §9.6 bans fabricated data in a
		// shipped surface and §7.2 Tier 0 bans the entrance animation by name.
		const said = HOME_SUMMARY_MARKUP.toLowerCase();
		for (const banned of ['skeleton', 'shimmer', 'placeholder', 'loadingnote', 'animation']) {
			expect(said).not.toContain(banned);
		}
	});
});
