/*
 * THE `/library` SCREEN, GUARDED.
 *
 * WHY IT READS SOURCE. `vitest.config.ts` is `environment: 'node'` with no
 * Svelte plugin, so a component cannot be imported and compiled in a test at
 * all. The repo's answer is already in the tree twice — `home.test.ts` reads a
 * template through Vite's `?raw` and runs a ban list over it, and
 * `havecell.test.ts` does the same for the Have column — so the same idiom is
 * used here for the rules that live in markup, and `recentEmptyState` is
 * imported and called directly for the one rule that does not.
 *
 * ⚠️ EVERY SLICE BELOW ASSERTS THAT IT FOUND SOMETHING BEFORE IT ASSERTS
 * ANYTHING ABOUT IT. The failure mode of a text guard is matching nothing at
 * all: an empty string passes every `not.toContain` there is, so a renamed
 * constant would turn each of these green rather than red.
 *
 * ⚠️ AND IT GUARDS THE ONE ROUTE INTO THE SCREEN AS WELL AS THE SCREEN. §17.8's
 * Libraries rows link here with a `?lib=` slug on them, and the two halves of
 * that link fail in opposite files: a row that stops linking is a dead end on
 * the Libraries screen, and a row that links to the wrong shape of address is a
 * wrong page on this one. Keeping both assertions together is what stops one of
 * them being relaxed to match a change to the other.
 */

import { describe, expect, it } from 'vitest';
import { branchMarkup, userFacingMarkup } from './copyguard';
import { findContentSizedTracks } from './list';
import {
	BROWSE_AZ_UNAVAILABLE,
	browseFeedFor,
	browseParams,
	browseSortNote,
	browseSortsFor,
	REFUSED_BROWSE_SORT,
	type BrowseFeed,
	type BrowseQuery
} from './librarygrid';
import { recentEmptyState } from './libraryscreen';
import ROUTE_SOURCE from '../routes/library/+page.svelte?raw';
import HOME_SOURCE from '../routes/+page.svelte?raw';
import LAYOUT_SOURCE from '../routes/+layout.svelte?raw';
import LIBRARIES_SOURCE from '../routes/libraries/+page.svelte?raw';

const ROUTE = 'routes/library/+page.svelte';
const MARKUP = userFacingMarkup(ROUTE_SOURCE);

/** One `<Tag ... />` off the stripped markup, or a failure that says which. */
function selfClosingTag(markup: string, tag: string): string {
	const open = markup.indexOf(`<${tag}`);
	expect(open, `${ROUTE} no longer renders <${tag}`).toBeGreaterThanOrEqual(0);
	const end = markup.indexOf('/>', open);
	expect(end, `${ROUTE}'s <${tag}> is not self-closing any more`).toBeGreaterThan(open);
	return markup.slice(open, end);
}

/** The `const NAME: … = [ … ];` initialiser, comments and all. */
function declaration(source: string, name: string): string {
	const open = source.indexOf(`const ${name}`);
	expect(open, `${ROUTE} no longer declares ${name}`).toBeGreaterThanOrEqual(0);
	const end = source.indexOf('];', open);
	expect(end, `${ROUTE}'s ${name} is not an array literal any more`).toBeGreaterThan(open);
	return source.slice(open, end);
}

const LIST_TAG = selfClosingTag(MARKUP, 'List');
const COLUMNS = declaration(ROUTE_SOURCE, 'COLUMNS');

describe('recentEmptyState', () => {
	/*
	 * An empty catalogue means three different things depending on what is
	 * connected, and a fourth when UsArr could not find out. The screen renders
	 * ONE of these into `<List emptyTitle>`, so a branch that returned the wrong
	 * one would tell a user with no service at all that an import is on its way,
	 * which is §17.7's own failure mode written as copy.
	 */
	it('gives each mode its own words', () => {
		const titles = [
			recentEmptyState('library').title,
			recentEmptyState('search-and-grab').title,
			recentEmptyState('unconfigured').title,
			recentEmptyState(undefined).title
		];
		expect(
			new Set(titles).size,
			`two modes share an empty-state title: ${titles.join(' / ')}`
		).toBe(4);
		for (const state of [
			recentEmptyState('library'),
			recentEmptyState('search-and-grab'),
			recentEmptyState('unconfigured'),
			recentEmptyState(undefined)
		]) {
			expect(state.title.length, 'an empty state has no title').toBeGreaterThan(0);
			expect(state.text.length, `${state.title} has no explanation under it`).toBeGreaterThan(0);
		}
	});

	/*
	 * The unknown case is NOT the "nothing catalogued yet" case, and the
	 * distinction is the whole reason it exists: a failed services read means
	 * UsArr does not know why the list is empty, and saying "an import has not
	 * run" on the strength of a read that did not answer is a claim about the
	 * library made out of a claim about the network.
	 */
	it('does not assert an import is pending when the services read failed', () => {
		const unknown = recentEmptyState(undefined);
		expect(unknown.title, 'the unknown state borrowed the connected-library title').not.toBe(
			recentEmptyState('library').title
		);
		expect(
			unknown.text,
			'the unknown state claims a library-bearing service is connected, which is exactly ' +
				'the fact the failed read did not establish'
		).not.toContain('is connected and');
	});

	/*
	 * ⚠️ THE DRIFT GUARD, AND IT IS THE POINT OF THIS FILE. Home's Block C and
	 * this screen draw the same table over the same endpoint. An install with a
	 * connected library and no rows yet must not be told two different stories
	 * depending on which screen it is read from, and nothing else in the tree
	 * would notice if it were: the two strings are four files apart.
	 */
	it('is word for word Home Block C, for the mode Home draws', () => {
		const title = /emptyTitle="([^"]+)"/.exec(HOME_SOURCE);
		const text = /emptyText="([^"]+)"/.exec(HOME_SOURCE);
		expect(
			title,
			'routes/+page.svelte no longer passes a literal emptyTitle to Block C, so this ' +
				'guard has nothing to compare against and must be rewritten rather than deleted'
		).not.toBeNull();
		expect(
			text,
			'routes/+page.svelte no longer passes a literal emptyText to Block C'
		).not.toBeNull();

		const state = recentEmptyState('library');
		expect(
			state.title,
			"Home's Block C and /library disagree about what an empty catalogue is called"
		).toBe(title?.[1]);
		expect(
			state.text,
			"Home's Block C and /library explain an empty catalogue differently. One of the " +
				'two was reworded; reconcile them rather than relaxing this test.'
		).toBe(text?.[1]);
	});
});

describe('the /library route draws the shared Have cell', () => {
	it('imports it and renders it', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer imports the shared Have cell from $lib/HaveCell.svelte`
		).toContain("from '$lib/HaveCell.svelte'");
		expect(
			MARKUP,
			`${ROUTE} imports the shared Have cell but never renders <HaveCell />. An import ` +
				'with no element is a column drawn by something else.'
		).toContain('<HaveCell');
	});

	it('has no inline availability chain of its own', () => {
		const inlined = ['complete', 'none', 'fraction', 'partial'].filter((m) =>
			MARKUP.includes(`mark.k === '${m}'`)
		);
		expect(
			inlined,
			`${ROUTE} tests the availability mark inline (${inlined.join(', ')}). That is the ` +
				'copy-paste $lib/HaveCell.svelte exists to end: two screens had already come ' +
				'apart on it once. Pass the row to <HaveCell /> and let it decide.'
		).toEqual([]);
		expect(
			MARKUP,
			`${ROUTE} calls haveCell() in its own markup. The component owns that call.`
		).not.toContain('haveCell(');
	});
});

describe('the /library table sorts server-side or not at all', () => {
	/*
	 * ⚠️ THIS DESCRIBE USED TO BE `offers no filter and no sort`, AND THE OLD
	 * REASONING IS WORTH KEEPING BECAUSE IT IS STILL THE REASON THE CONTROLS
	 * TAKE THE SHAPE THEY DO. What the screen holds in the DOM is a keyset PREFIX
	 * of the catalogue — the newest N rows, for whatever N `Load more` has been
	 * pressed up to — so a control that sorted or filtered THOSE rows would
	 * present itself as operating on the library and would in fact operate on the
	 * prefix: "no comics found" would mean "no comics in the newest 200 rows".
	 *
	 * The screen now reads `GET /api/v1/library`, which applies the order and the
	 * `?lib=` scope IN SQL over the whole table, so the control is honest. What
	 * has NOT changed is that a client-side one would not be, and the guards
	 * below are that distinction: the sort is a URL parameter the server acts on,
	 * and nothing sorts or filters rows in the browser.
	 */
	it('declares no sortable column', () => {
		expect(
			COLUMNS,
			`${ROUTE} declares a sortable column. A header sort would sort the keyset prefix ` +
				'in the DOM and claim to have sorted the library.'
		).not.toContain('sortable');
	});

	it('wires no sort state into the list', () => {
		for (const prop of ['sortKey', 'sortDir', 'onsort']) {
			expect(
				LIST_TAG,
				`${ROUTE} passes ${prop} to <List>. The primitive draws a sort affordance from ` +
					'it, and there is nothing behind that affordance but a prefix.'
			).not.toContain(prop);
		}
	});

	it('ships no client-side filter over the keyset prefix', () => {
		for (const control of ['<input', 'filteredEmpty']) {
			expect(
				MARKUP,
				`${ROUTE} renders ${control}. A filter over a keyset prefix answers a question ` +
					'about the newest N rows in the words of a question about the library, and ' +
					'the endpoint offers no text filter to apply server-side either.'
			).not.toContain(control);
		}
	});

	/*
	 * ⚠️ THE ORDER GOES TO THE SERVER, AND THAT IS THE WHOLE DIFFERENCE BETWEEN
	 * THIS CONTROL AND THE ONE THE OLD HEADER REFUSED. `onSort` writes `?sort=`
	 * into the address; `$lib/librarygrid`'s `browseParams` puts it on the wire
	 * and `browseFeedFor` drops the feed so the next page is read from the start
	 * of the new order. A control that reordered `feed.items` in place would look
	 * identical on a first page and be wrong on every page after it.
	 */
	it('puts the order in the address rather than in the DOM', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer writes ?sort= into the address, so a sorted view is not ` +
				'linkable and cannot survive a reload'
		).toContain("params.set('sort', value)");
		expect(
			ROUTE_SOURCE,
			`${ROUTE} sorts rows it already holds. That sorts the keyset prefix and claims ` +
				'to have sorted the library.'
		).not.toMatch(/\.items\.(sort|toSorted)\(/);
	});

	/*
	 * ⚠️ THE OPTIONS ARE `browseSortsFor`'s AND ARE NEVER SPELT OUT IN MARKUP.
	 * The store refuses `sort_title` when the corpus is not exactly one
	 * `work.kind`, and an all-types view is six — so a hard-coded option list
	 * here would offer A to Z and collect a 400. §17.1 wants a NATIVE control,
	 * so it is a `<select>` rather than a custom listbox.
	 */
	it('draws a native select whose options the module decides', () => {
		expect(MARKUP, `${ROUTE} no longer renders a native <select> for the order`).toContain(
			'<select'
		);
		expect(
			MARKUP,
			`${ROUTE} no longer builds the options from browseSortsFor(), so the control can ` +
				'offer an order the server refuses'
		).toMatch(/#each sorts as sort/);
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer derives its options from browseSortsFor(query.mediaType). That ` +
				"function is the store's own len(kinds) != 1 and is what keeps A to Z off an " +
				'all-types control.'
		).toContain('browseSortsFor(query.mediaType)');
	});
});

describe('the /library table tells ARIA the truth about its size', () => {
	/*
	 * ARIA defines `aria-rowcount="-1"` for a total that is genuinely unknown,
	 * and it is unknown here by construction: a keyset endpoint never says how
	 * many rows exist. Passing the rendered count while a cursor is outstanding
	 * is what makes a screen reader say "row 3 of 200" when the truth is "row 3
	 * of 4,000", and it is what would put "200 of 200" under a Load-more button
	 * with thousands of rows left to fetch. `List.svelte` says so at the prop.
	 */
	it('passes total only once the feed is exhausted', () => {
		const total = /total=\{([^}]*)\}/.exec(LIST_TAG);
		expect(total, `${ROUTE} no longer passes total to <List>`).not.toBeNull();
		expect(
			total?.[1],
			`${ROUTE} passes total={${total?.[1] ?? ''}} unconditionally. While a cursor is ` +
				'outstanding the total is unknown, and the honest value is `undefined`.'
		).toContain('undefined');
	});
});

describe('the /library table pages to the end of the catalogue', () => {
	it('reads "is there more" off the cursor rather than off a short page', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer asks nextBrowsePage() for the next page. The stop rule lives ` +
				'there and nowhere else: the server issues a second statement for the undated ' +
				'tail, so a short page is legal and is not the end of the list.'
		).toContain('nextBrowsePage(');
		expect(
			ROUTE_SOURCE,
			`${ROUTE} compares a page length against the page size. That is the stop rule ` +
				'$lib/library exists to keep out of a template: it truncates the table at the ' +
				'first row whose upstream reported no creation date, silently and for ever.'
		).not.toMatch(/length\s*[<>]=?\s*LOAD_MORE_PAGE_SIZE/);
	});
});

describe('the /library table can line its columns up', () => {
	/*
	 * ADR-0029 makes every row an independent grid, so a content-sized track
	 * resolves against its OWN row's contents and the header cannot agree with
	 * the body. It shipped once and nothing failed: it rendered, it just
	 * misaligned. `findContentSizedTracks` is the running app's own validator,
	 * called here on the widths this route actually declares.
	 */
	it('declares no content-sized track', () => {
		const widths = [...COLUMNS.matchAll(/width:\s*'([^']+)'/g)].map((m) => m[1]);
		expect(
			widths.length,
			`no column widths were found in ${ROUTE}'s COLUMNS, so this check is asserting ` +
				'nothing at all'
		).toBe(5);
		expect(
			findContentSizedTracks(widths),
			`${ROUTE} declares a content-sized track. Every row is its own grid, so the header ` +
				'sizes to the column name and each body row sizes to its own contents.'
		).toEqual([]);
	});
});

describe('the /library screen is reachable', () => {
	/*
	 * A screen with no route into it is a screen that does not exist. THIS entry
	 * is hand-written rather than derived — unlike the six media-type rows, which
	 * `TYPE_NAV` builds from `MEDIA_TYPES` and titles from the same list — so its
	 * entry and its title are two facts that must agree; a row with no title
	 * renders an empty toolbar and an empty h1, which is the shape of the bug
	 * this pair catches.
	 */
	it('has a sidebar entry and a title', () => {
		expect(
			LAYOUT_SOURCE,
			'routes/+layout.svelte has no /library entry in NAV_GROUPS, so nothing in the ' +
				'application links to the screen'
		).toContain("id: '/library'");
		expect(
			LAYOUT_SOURCE,
			'routes/+layout.svelte has no /library title, so the toolbar and the h1 render empty'
		).toContain("[resolve('/library')");
	});
});

describe('the /library screen is the ALL-TYPES scoped view', () => {
	/*
	 * ⚠️ THE SCREEN READS THE BROWSE ENDPOINT, NOT `/library/recent`, and that is
	 * the switch this whole block exists to hold. `/library/recent` takes no
	 * `lib` and no `sort` — `handleRecentWorks` parses `limit` and `cursor` and
	 * nothing else — so a sort control or a scope over it could only ever have
	 * been a control over the keyset prefix in the DOM.
	 *
	 * The switch was safe because the two reads AGREE at the default: no `lib`,
	 * no `media_type`, `sort=added_at` gives the same six `work.kind` values in
	 * the same `added_at DESC, id DESC` order with the same undated-tail handoff.
	 * `TestBrowseWorksUnfilteredIsBlockCsCorpus` in `internal/store/browse_test.go`
	 * walks every page of the unfiltered browse and asserts it equals Block C's
	 * order, undated rows included. This guard holds the client half of it.
	 */
	it('reads the browse endpoint and not the recent one', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer reads the browse endpoint, so its sort control and its ?lib= ` +
				'scope have nothing behind them'
		).toContain('fetchBrowsePage(');
		expect(
			ROUTE_SOURCE,
			`${ROUTE} reads /library/recent again. That endpoint takes no lib and no sort, so ` +
				'both controls on this screen would be operating on the keyset prefix.'
		).not.toContain('fetchRecentPage(');
	});

	/*
	 * ⚠️ NO `media_type` IS THE POINT OF THE SCREEN. Six per-type screens already
	 * exist at `/library/[type]`; this one is the view that filters by no type at
	 * all, which is what makes it safe for a library scope to land on — a library
	 * can span media types, so a type filter here would drop part of the library
	 * a `?lib=` address names.
	 *
	 * ⚠️ THE AUDIOBOOKSHELF EXAMPLE WAS CITED FOR THAT AND SHOWS THE OPPOSITE.
	 * §17.8 offers one ABS `mediaType=book` container as *two* libraries, Ebooks
	 * and Audiobooks — each of the two is SINGLE-type, which is the improvement
	 * over ABS's own organisation rather than an instance of spanning. (It is
	 * also not v0.1: §17.8 dates it to the milestone Audiobookshelf lands in.)
	 * What does support this rule is §17.8's `Kind` control, which is labelled
	 * `Movies · TV · Music · Books · Comics` and carries a help line under Books:
	 * *"Books covers ebooks and audiobooks. The format filter below decides which
	 * this library holds."* A `Books` library with no format filter therefore
	 * holds two of §17.2's six media types, and a per-type link into it drops
	 * one of them.
	 */
	it('sends no media_type of its own', () => {
		expect(
			ROUTE_SOURCE,
			`${ROUTE} resolves a media type. This screen is the all-types view; a type filter ` +
				'on it would hide part of any library that spans types.'
		).not.toContain('browseRoute(');
		expect(
			ROUTE_SOURCE,
			`${ROUTE} no longer resolves its address through browseAllTypesRoute(), which is ` +
				'what applies §7.3s 32-slug bound before anything is sent'
		).toContain('browseAllTypesRoute(');
	});
});

describe('A to Z is stated as unavailable, in UsArr own words', () => {
	/*
	 * ⚠️ NEVER OFFERED AND THEN REFUSED. `sort_title` walks `ix_work_kind_sort`,
	 * which is `(kind, sort_title, id)`, and SQLite cannot supply ORDER BY from
	 * an index whose leading column is constrained by IN — so the order needs
	 * exactly ONE `work.kind` and this corpus is six. That is knowable before a
	 * request is sent, so the option is absent from the control and the reason is
	 * printed beside it.
	 */
	it('offers added_at and popularity and neither sort_title nor year', () => {
		const offered = browseSortsFor(undefined);
		expect(offered, 'the all-types sort control lost its default order').toContain('added_at');
		expect(offered, 'the all-types sort control lost popularity').toContain('popularity');
		expect(
			offered,
			'the all-types sort control offers sort_title. The store refuses it whenever the ' +
				'corpus is not exactly one work.kind, so this option collects a 400.'
		).not.toContain('sort_title');
		expect(
			offered,
			'the all-types sort control offers year. work.year has no index at all and the ' +
				'endpoint refuses it by name.'
		).not.toContain(REFUSED_BROWSE_SORT);
	});

	/*
	 * ⚠️ THE CONDITION IS THE KIND COUNT AND NOT THE LIBRARY SCOPE. A scope
	 * narrows rows and changes no index, so a rule keyed on `?lib=` would fire on
	 * the wrong condition and still look right on the screen it was written for.
	 */
	it('is decided by the kind count and not by the scope', () => {
		const scoped = browseSortsFor(undefined);
		expect(
			browseSortNote({ sort: 'added_at', libraries: [] }),
			'the A to Z note went missing on an unscoped all-types view, so the absence of the ' +
				'option is now unexplained'
		).toBe(BROWSE_AZ_UNAVAILABLE);
		expect(
			browseSortNote({ sort: 'added_at', libraries: ['films', 'books'] }),
			'the A to Z note changed with the library scope. A scope changes no index, so the ' +
				'note must not be keyed on it.'
		).toBe(BROWSE_AZ_UNAVAILABLE);
		expect(
			browseSortsFor(undefined),
			'browseSortsFor stopped being a pure function of the media type'
		).toEqual(scoped);
		expect(
			browseSortNote({ mediaType: 'movies', sort: 'added_at', libraries: [] }),
			'a single-kind type lost A to Z, or gained the all-types note that would tell it ' +
				'this view spans every media type when it spans exactly one'
		).toBeUndefined();
	});

	/*
	 * ⚠️ THE SERVER'S 400 TEXT MUST NOT BE THE THING THE READER SEES.
	 * `handleBrowseWorks` answers an unservable all-types alphabetical sort with
	 * "add a media_type — movies, tv, ebooks, audiobooks or comics — to sort by
	 * title, or sort by added_at or popularity, which work across every type at
	 * once". It names wire parameters and sort keys to a reader who is choosing
	 * from a control, not writing a query string. Correct for a wire consumer,
	 * wrong for this audience.
	 *
	 * ⚠️ AND THE ABSENCE ASSERTION IS PRECEDED BY A PRESENCE ONE, because an
	 * absence assertion over an empty string passes and proves nothing: the note
	 * has to exist and be a sentence before "these words are not in it" means
	 * anything at all.
	 */
	it('renders none of the words the server 400 uses', () => {
		expect(
			BROWSE_AZ_UNAVAILABLE.length,
			'the A to Z note is empty, so every absence assertion below passes over nothing'
		).toBeGreaterThan(20);
		expect(
			MARKUP,
			`${ROUTE} no longer renders the note, so the missing option is unexplained on screen`
		).toContain('{sortNote}');

		// The distinctive tokens of the server's own sentence. Each is a fragment
		// of `internal/httpapi/library.go`'s ErrUnservableSort arm, quoted from it
		// rather than paraphrased.
		for (const leak of [
			'sort_title',
			'media_type',
			'no index',
			'music',
			'year',
			'added_at',
			'popularity'
		]) {
			expect(
				BROWSE_AZ_UNAVAILABLE.toLowerCase(),
				`the A to Z note contains "${leak}", which is the server's own 400 wording ` +
					'reaching a reader. That sentence is addressed to whoever built the query ' +
					'string and names wire parameters and sort keys, not anything on screen.'
			).not.toContain(leak);
		}
	});
});

describe('a Libraries row leads to its own scoped view', () => {
	const LIBRARIES_ROUTE = 'routes/libraries/+page.svelte';

	/*
	 * ⚠️ TWO CORPORA OFF ONE FILE, BECAUSE THE RULES BELOW ARE ABOUT DIFFERENT
	 * HALVES OF IT. `resolve('/library')` and `libraryScopeHref` live in the
	 * `<script>`, which `userFacingMarkup` deletes, so those read the raw source;
	 * the Library cell is markup, and reading it raw would hand the guard the
	 * cell's own explanatory comment as if it were rendered text.
	 */
	const LIBRARIES_MARKUP = userFacingMarkup(LIBRARIES_SOURCE);

	/**
	 * The Library cell's arm of `{#snippet cell}`, bounded to where the `kind` arm
	 * opens rather than to a character count. See `branchMarkup`: the count this
	 * replaces overran the arm by 321 characters into three of its siblings.
	 */
	const cellMarkup = () => branchMarkup(LIBRARIES_MARKUP, "column.id === 'library'");

	/** The `libraryScopeHref` body, which is where the whole decision sits. */
	const SCOPE_HREF = (() => {
		const open = LIBRARIES_SOURCE.indexOf('function libraryScopeHref');
		expect(
			open,
			`${LIBRARIES_ROUTE} no longer builds a scoped link, so its rows lead nowhere`
		).toBeGreaterThanOrEqual(0);
		const end = LIBRARIES_SOURCE.indexOf('\n\t}', open);
		expect(end, 'libraryScopeHref is no longer a function body').toBeGreaterThan(open);
		return LIBRARIES_SOURCE.slice(open, end);
	})();

	/*
	 * ⚠️ THE ALL-TYPES VIEW, NEVER A PER-TYPE GRID. §17.8's `Kind: Books` covers
	 * ebooks and audiobooks together — its own help line says so, and only the
	 * format filter narrows it — so a row that led to `/library/ebooks?lib=…`
	 * would silently drop every audiobook in an unfiltered Books library it
	 * claims to open, and the screen would look correct doing it. ⚠️ THIS CITED
	 * §17.8's AUDIOBOOKSHELF EXAMPLE, WHICH SHOWS THE REVERSE: one upstream
	 * container offered as two libraries, each of them single-type.
	 */
	it('links to /library and not to a per-type grid', () => {
		expect(
			SCOPE_HREF,
			`${LIBRARIES_ROUTE} builds its row link with the slug interpolated straight into a ` +
				'string. Build it with URLSearchParams so escaping is a property of the code.'
		).toContain('URLSearchParams');
		expect(
			SCOPE_HREF,
			`${LIBRARIES_ROUTE}'s row link no longer carries the library as ?lib=`
		).toContain('lib: scoped');
		expect(
			LIBRARIES_SOURCE,
			`${LIBRARIES_ROUTE} resolves a per-type route for its rows. A library spans media ` +
				'types, so choosing one on the user behalf hides the rest of the library.'
		).not.toContain("resolve('/library/[type]'");
		expect(
			LIBRARIES_SOURCE,
			`${LIBRARIES_ROUTE} no longer resolves /library, so the base path of the link is ` +
				'not carried and a configured base path breaks it'
		).toContain("resolve('/library')");
	});

	/*
	 * ⚠️ AN EMPTY `lib` IS A 400 AND NOT "no scope". The server tests PRESENCE,
	 * so `?lib=`, `?lib=%20` and `?lib=,,` are all refusals — which means a row
	 * whose slug did not parse must get NO LINK rather than a link to a refusal.
	 * `libraries.ts` reads the field with `str()`, so a missing or non-string
	 * slug arrives as `''` and is exactly the case this covers.
	 *
	 * ⚠️ AND THIS GUARD USED TO PIN THE WRONG KEY. It asserted the literal
	 * `if (slug === '') return undefined`, which is EMPTINESS — a proxy that
	 * passes `'   '` straight through into `?lib=%20`. What the link actually
	 * depends on is whether `readLibraryScope` RESOLVES the slug at the far end,
	 * so the gate is `scopeSlug` (`librarygrid.ts`) and this pins that instead.
	 * `librarygrid.test.ts` asserts the resolution itself; here the only question
	 * is that the screen keys on it, and that the proxy has not come back.
	 */
	it('emits no empty lib, and gives such a row no link at all', () => {
		expect(
			SCOPE_HREF,
			`${LIBRARIES_ROUTE} builds a ?lib= link for a library with no slug. An empty lib ` +
				'is a 400, so that row would link to a refusal.'
		).toContain('scopeSlug(slug)');
		expect(
			SCOPE_HREF,
			`${LIBRARIES_ROUTE} gates the row link on the slug being non-empty again. A blank ` +
				'slug is non-empty and resolves to no scope, so that row would link to every library.'
		).not.toContain("slug === ''");
		const cell = cellMarkup();
		expect(
			cell.length,
			`${LIBRARIES_ROUTE}'s Library cell arm is down to a stub, so the rule below is ` +
				'being asserted over a branch that no longer draws anything'
		).toBeGreaterThan(120);
		expect(
			cell,
			`${LIBRARIES_ROUTE} renders the Library cell without the undefined-href branch, so ` +
				'a slugless row either links to a 400 or renders a broken anchor'
		).toContain('href === undefined');
	});

	/*
	 * §17.1: a real `<a href>`, because that is what middle-click, Ctrl-click and
	 * "copy link address" act on. A click handler calling `goto` would look
	 * identical on screen and break all three.
	 */
	it('is a real anchor rather than a click handler', () => {
		const cell = cellMarkup();
		expect(
			cell.length,
			`${LIBRARIES_ROUTE}'s Library cell arm is down to a stub, so the rule below is ` +
				'being asserted over a branch that no longer draws anything'
		).toBeGreaterThan(120);
		expect(cell, `${LIBRARIES_ROUTE}'s Library cell is no longer an <a> carrying the href`).toMatch(
			/<a class="trunc" \{href\}/
		);
	});
});

describe('the cursor is dropped whenever the query moves', () => {
	/*
	 * ⚠️ NOTHING SERVER-SIDE WOULD CATCH THIS. A browse cursor binds to `sort`
	 * ALONE: replaying one under a different sort is a loud 400, but replaying it
	 * under a different `?lib=` is a `200 OK` whose page starts partway into a
	 * different corpus — rows skipped, count wrong, no symptom anywhere. The rule
	 * is `browseFeedFor`'s and it drops the whole feed rather than the cursor
	 * alone, because keeping the rows would show one scope's items under
	 * another's heading.
	 */
	it('drops the feed on a sort change and on a scope change', () => {
		const base: BrowseQuery = { sort: 'added_at', libraries: ['films'] };
		const feed: BrowseFeed = {
			items: [],
			cursor: '1aXYZ',
			loaded: true,
			limit: 50,
			query: base
		};
		expect(
			browseFeedFor(feed, base).cursor,
			'the feed was dropped although nothing about the query changed, so every page ' +
				're-reads page one'
		).toBe('1aXYZ');
		expect(
			browseFeedFor(feed, { ...base, sort: 'popularity' }).cursor,
			'a sort change kept the cursor. The server refuses it, so the screen would show a ' +
				'400 instead of the order that was asked for.'
		).toBeUndefined();
		expect(
			browseFeedFor(feed, { ...base, libraries: ['books'] }).cursor,
			'a SCOPE change kept the cursor. The server accepts it with a 200 and serves a page ' +
				'from partway into a different corpus, which has no symptom at all.'
		).toBeUndefined();
		expect(
			browseFeedFor(feed, { ...base, libraries: [] }).items,
			'clearing the scope kept the rows read under it, so a library scope stays on screen ' +
				'after it was removed from the address'
		).toEqual([]);
	});

	/*
	 * The screen half: `browseParams` must never spell an empty scope as `?lib=`,
	 * because that is the one parameter whose empty form is a refusal rather than
	 * an absence.
	 */
	it('never puts an empty lib on the wire', () => {
		const unscoped = browseParams({ sort: 'added_at', libraries: [] }, { limit: 50 });
		expect(
			unscoped.has('lib'),
			'browseParams emitted lib on an unscoped query. `?lib=` is a 400: the server tests ' +
				'presence, not emptiness.'
		).toBe(false);
		expect(
			unscoped.has('media_type'),
			'browseParams emitted media_type on the all-types query. It is omitted rather than ' +
				'sent empty, so one rule covers both filters.'
		).toBe(false);
		expect(unscoped.get('sort'), 'browseParams stopped sending the order').toBe('added_at');
		expect(
			browseParams({ sort: 'added_at', libraries: ['films', 'books'] }, { limit: 50 }).get('lib'),
			'browseParams stopped joining the scope, so a multi-library address loses libraries'
		).toBe('films,books');
	});
});
