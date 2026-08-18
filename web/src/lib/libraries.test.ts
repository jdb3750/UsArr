import { describe, expect, it } from 'vitest';
import { ApiError, type ServiceHealth } from './api';
import {
	HEALTH_UNREAD,
	LIBRARIES_URL,
	describeFailure,
	healthNote,
	healthRead,
	itemCountText,
	kindLabel,
	libraryStates,
	sourceChips,
	sourceVerdict,
	toFormats,
	toLibraries,
	toLibrary,
	toLibrarySource,
	unboundNote,
	unboundServices,
	type HealthRead,
	type Library,
	type LibrarySource
} from './libraries';
import { stateLabel } from './services';

/**
 * §17.8's ROW VIEW, PINNED — over the wire `internal/httpapi/libraries.go`
 * actually marshals and the contract `docs/reference/http-api.md` §2 writes
 * down.
 *
 * Every fixture below is shaped from the server's own source rather than from a
 * summary of it: `libraryResponse` and `librarySourceResponse` for the two
 * structs, §2.2's worked example for the optional keys, and §2.4 for the three
 * fields that describe states nothing in the tree can currently reach.
 *
 * AND THE SECOND READ, `GET /api/v1/services/health` (§3), which is where the
 * per-source health §17.8's row view asks for actually lives. Two properties are
 * pinned everywhere below rather than in one test:
 *
 *   · NEITHER READ BLOCKS THE OTHER. Every shaping function takes a
 *     `HealthRead`, and `HEALTH_UNREAD` is a first-class value rather than an
 *     error: on it the screen renders exactly what it rendered before the join
 *     existed. The suites that predate the join now pass it explicitly, which is
 *     what makes them that regression.
 *   · ABSENCE IS NEVER HEALTH. A source naming an instance the health read does
 *     not mention is `unlisted`, an instance mentioned but unprobed is
 *     `unchecked`, and neither is ever `connected`.
 */

/** One source as `librarySourceResponse` marshals it. */
function wireSource(over: Record<string, unknown> = {}): Record<string, unknown> {
	return {
		id: 2,
		service_instance_id: 1,
		service_name: 'Kavita Manga',
		service_kind: 'kavita',
		container_kind: 'remote_library',
		container_ref: '12',
		container_name: 'Books',
		is_metadata_authority: true,
		...over
	};
}

/** One row as `libraryResponse` marshals it. */
function wireLibrary(over: Record<string, unknown> = {}): Record<string, unknown> {
	return {
		id: 2,
		name: 'Ebooks',
		slug: 'ebooks',
		kind: 'book',
		sort_order: 5,
		enabled: true,
		include_in_search: true,
		item_count: 3,
		sources: [wireSource()],
		...over
	};
}

/** A parsed row, for the shaping functions that take one. */
function library(over: Partial<Library> = {}): Library {
	const parsed = toLibrary(wireLibrary());
	if (parsed === undefined) throw new Error('the base fixture must parse');
	return { ...parsed, ...over };
}

/** A parsed source, for the shaping functions that take one. */
function source(over: Partial<LibrarySource> = {}): LibrarySource {
	const parsed = toLibrarySource(wireSource());
	if (parsed === undefined) throw new Error('the source fixture must parse');
	return { ...parsed, ...over };
}

/**
 * One health row, as `$lib/api`'s `ServiceHealth` holds it after parsing.
 *
 * The default is the ordinary case a screen must NOT read as remarkable: an
 * enabled, healthy, freshly probed instance that has completed an import. Every
 * fixture below is that row with one field moved, so each test says which single
 * fact it is about.
 */
function healthRow(over: Partial<ServiceHealth> = {}): ServiceHealth {
	return {
		id: 1,
		name: 'Kavita Manga',
		kind: 'kavita',
		role: 'library',
		baseUrl: 'http://kavita.example',
		enabled: true,
		state: 'healthy',
		breakerState: 'closed',
		consecutiveFailures: 0,
		warnings: [],
		blockedIndexers: [],
		stale: false,
		lastFullSyncAt: '2026-08-17T12:00:00Z',
		workCount: 3,
		...over
	};
}

/** The health read, from a list of rows. */
function read(...rows: ServiceHealth[]): HealthRead {
	return healthRead(rows);
}

/** The default read: one healthy Kavita, which is instance 1 in every fixture. */
const CONNECTED = read(healthRow());

describe('the endpoint', () => {
	it('is the one internal/httpapi/server.go routes', () => {
		expect(LIBRARIES_URL).toBe('/api/v1/libraries');
	});

	it('takes no query parameters, so the URL is a constant', () => {
		// §2.1: "None, and there is no paging." A client that appended a limit or a
		// cursor would be coding against an endpoint that has neither.
		expect(LIBRARIES_URL).not.toContain('?');
	});
});

describe('one source off the wire', () => {
	it('reads every field librarySourceResponse publishes', () => {
		expect(toLibrarySource(wireSource())).toEqual({
			id: 2,
			serviceInstanceId: 1,
			serviceName: 'Kavita Manga',
			serviceKind: 'kavita',
			containerKind: 'remote_library',
			containerRef: '12',
			containerName: 'Books',
			isMetadataAuthority: true
		});
	});

	it('leaves the two optional keys OFF rather than undefined-valued', () => {
		// §2.2's second worked example: "The SAME shape with the optional keys
		// absent. A healthy source has no `missing_since`; this is not an error."
		const source = toLibrarySource(
			wireSource({ id: 3, container_name: undefined, is_metadata_authority: false })
		);
		expect(source).toBeDefined();
		expect(Object.hasOwn(source ?? {}, 'containerName')).toBe(false);
		expect(Object.hasOwn(source ?? {}, 'missingSince')).toBe(false);
	});

	it('keeps missing_since when the server sends it', () => {
		const source = toLibrarySource(wireSource({ missing_since: '2026-08-17T09:30:00Z' }));
		expect(source?.missingSince).toBe('2026-08-17T09:30:00Z');
	});

	it('drops a frame with no id, because the chip has no key', () => {
		expect(toLibrarySource(wireSource({ id: undefined }))).toBeUndefined();
		expect(toLibrarySource('not an object')).toBeUndefined();
	});

	it('keeps a source whose name did not travel', () => {
		// Dropping it would make the library read as MORE orphaned than it is,
		// which is the one inference this screen must not make.
		const source = toLibrarySource(wireSource({ service_name: undefined }));
		expect(source).toBeDefined();
		expect(source?.serviceName).toBe('');
	});
});

describe('one library off the wire', () => {
	it('reads every field libraryResponse publishes', () => {
		expect(toLibrary(wireLibrary())).toEqual({
			id: 2,
			name: 'Ebooks',
			slug: 'ebooks',
			kind: 'book',
			sortOrder: 5,
			enabled: true,
			includeInSearch: true,
			itemCount: 3,
			sources: [
				{
					id: 2,
					serviceInstanceId: 1,
					serviceName: 'Kavita Manga',
					serviceKind: 'kavita',
					containerKind: 'remote_library',
					containerRef: '12',
					containerName: 'Books',
					isMetadataAuthority: true
				}
			]
		});
	});

	it('leaves formats and orphaned_at OFF, which is every row today', () => {
		// §2.4: neither column has a writer, so absence is the honest majority
		// state rather than a parse failure.
		const parsed = toLibrary(wireLibrary());
		expect(Object.hasOwn(parsed ?? {}, 'formats')).toBe(false);
		expect(Object.hasOwn(parsed ?? {}, 'orphanedAt')).toBe(false);
	});

	it('parses formats when something does start writing the column', () => {
		expect(toLibrary(wireLibrary({ formats: ['ebook'] }))?.formats).toEqual(['ebook']);
		expect(toFormats(['ebook', 'audiobook'])).toEqual(['ebook', 'audiobook']);
	});

	it('treats a formats value that is not a list of strings as no filter', () => {
		// The server already drops and logs a stored value that will not decode as
		// an array of strings, so a shape arriving here is one neither side can
		// render. Inventing a filter out of it would be worse than showing none.
		expect(toFormats('ebook')).toBeUndefined();
		expect(toFormats([])).toBeUndefined();
		expect(toFormats([1, 2])).toBeUndefined();
	});

	it('reads a library with ZERO sources, which is §17.8s orphaned state', () => {
		// §2.2's second row: "no sources left, never auto-deleted, shown with its
		// reason. `sources` is `[]` and never absent."
		const parsed = toLibrary(
			wireLibrary({
				id: 3,
				name: 'Loose Ends',
				slug: 'loose-ends',
				item_count: 0,
				orphaned_at: '2026-08-17T08:00:00Z',
				sources: []
			})
		);
		expect(parsed?.sources).toEqual([]);
		expect(parsed?.orphanedAt).toBe('2026-08-17T08:00:00Z');
	});

	it('reads an absent sources key as no sources rather than as undefined', () => {
		// §2.2 guarantees the key, so this frame is broken. The renderer must still
		// get an array: handing it `undefined` is how an `{#each}` throws.
		expect(toLibrary(wireLibrary({ sources: undefined }))?.sources).toEqual([]);
	});

	it('reads an absent enabled or include_in_search as false', () => {
		// Neither has `omitempty`, so an absent key is a broken frame. False
		// UNDERSTATES what the library does rather than claiming it is live and
		// searched when nothing said so.
		const parsed = toLibrary(wireLibrary({ enabled: undefined, include_in_search: undefined }));
		expect(parsed?.enabled).toBe(false);
		expect(parsed?.includeInSearch).toBe(false);
	});

	it('drops a frame with no id, because the row has no key', () => {
		expect(toLibrary(wireLibrary({ id: undefined }))).toBeUndefined();
	});

	it('keeps a row whose name did not travel', () => {
		// The cell has a rendering for an empty name; dropping the row would make
		// the table silently short by rows the server sent.
		expect(toLibrary(wireLibrary({ name: undefined }))?.name).toBe('');
	});
});

describe('the list', () => {
	it('preserves the server order and does not re-sort', () => {
		// `internal/store/libraries.go` ends `ORDER BY l.sort_order, l.name`. A
		// second implementation of that rule is a second thing that can disagree
		// with it, and the two WOULD disagree on the tie-break: the column is plain
		// TEXT, so SQLite compares it BINARY, where 'Z' sorts before 'a'.
		const payload = {
			items: [
				wireLibrary({ id: 7, name: 'Zines', sort_order: 1 }),
				wireLibrary({ id: 8, name: 'audiobooks', sort_order: 1 }),
				wireLibrary({ id: 9, name: 'Comics', sort_order: 0 })
			]
		};
		expect(toLibraries(payload).map((l) => l.name)).toEqual(['Zines', 'audiobooks', 'Comics']);
	});

	it('reads an empty install as an empty list', () => {
		// §2.2: `items` is always present and is `[]` on an empty install, so that
		// a zero state and a failure are not the same thing on the wire.
		expect(toLibraries({ items: [] })).toEqual([]);
	});

	it('reads a payload that is not the documented shape as an empty list', () => {
		// The same answer as an empty install, deliberately: §2.2 makes those two
		// indistinguishable on purpose, and a client inventing a third reading
		// would be inventing.
		expect(toLibraries(undefined)).toEqual([]);
		expect(toLibraries({})).toEqual([]);
		expect(toLibraries({ items: 'nope' })).toEqual([]);
	});

	it('drops only the frames it cannot key, keeping the rest', () => {
		const payload = { items: [wireLibrary(), { name: 'no id' }, wireLibrary({ id: 4 })] };
		expect(toLibraries(payload).map((l) => l.id)).toEqual([2, 4]);
	});
});

describe('the Kind column speaks the product vocabulary', () => {
	it('renders §17.8s five labels', () => {
		// §17.8: "Label it `Movies · TV · Music · Books · Comics`, let the schema
		// value be the value", and "The list's `Kind` column follows the same
		// labels".
		expect(kindLabel('movie')).toBe('Movies');
		expect(kindLabel('series')).toBe('TV');
		expect(kindLabel('artist')).toBe('Music');
		expect(kindLabel('book')).toBe('Books');
		expect(kindLabel('comic')).toBe('Comics');
	});

	it('renders the two schema kinds §17.8 gives no word to VERBATIM', () => {
		// `library.kind`'s CHECK admits seven values; §17.8 names five. A guess at
		// the other two is a wrong label the user cannot see through.
		expect(kindLabel('album')).toBe('album');
		expect(kindLabel('game')).toBe('game');
	});

	it('never renders a nothing-word for a kind it has not met', () => {
		expect(kindLabel('holodeck')).toBe('holodeck');
	});
});

describe('the Items column', () => {
	it('groups the figure', () => {
		expect(itemCountText(library({ itemCount: 1842 }))).toBe('1,842');
	});

	it('prints a real zero rather than a nothing-word', () => {
		// Zero items is a fact the State column then explains. `—` would hide it.
		expect(itemCountText(library({ itemCount: 0 }))).toBe('0');
	});
});

describe('the Sources cell', () => {
	function sourceAt(id: number, over: Record<string, unknown> = {}) {
		const parsed = toLibrarySource(wireSource({ id, ...over }));
		if (parsed === undefined) throw new Error('the source fixture must parse');
		return parsed;
	}

	it('renders one chip per source, labelled with the instance name', () => {
		const chips = sourceChips(library(), HEALTH_UNREAD);
		expect(chips.total).toBe(1);
		expect(chips.more).toBe(0);
		expect(chips.shown[0].label).toBe('Kavita Manga');
		expect(chips.shown[0].serviceInstanceId).toBe(1);
	});

	it("puts the upstream's own container name on the title, not in the label", () => {
		// §17.8 wants the upstream's own name for the container; the row view has
		// no second line for it, so it rides the `title` the truncation already
		// needs (§9.1 tier 1).
		expect(sourceChips(library(), HEALTH_UNREAD).shown[0].title).toBe('Kavita Manga · Books');
	});

	it('falls back to a title of just the label when no container name travelled', () => {
		const one = library({ sources: [sourceAt(2, { container_name: undefined })] });
		expect(sourceChips(one, HEALTH_UNREAD).shown[0].title).toBe('Kavita Manga');
	});

	it('names a source whose service name did not travel rather than dropping it', () => {
		const one = library({ sources: [sourceAt(2, { service_name: undefined })] });
		expect(sourceChips(one, HEALTH_UNREAD).shown[0].label).toBe('Unnamed service');
	});

	it('caps at three plus a count, per §9.1', () => {
		const many = library({ sources: [1, 2, 3, 4, 5].map((n) => sourceAt(n)) });
		const chips = sourceChips(many, HEALTH_UNREAD);
		expect(chips.shown).toHaveLength(3);
		expect(chips.more).toBe(2);
		expect(chips.total).toBe(5);
	});

	it('hoists a missing source in front of the cap', () => {
		// A cell that hid the one broken source behind `+2 more` would have dropped
		// the only fact on the row worth acting on.
		const many = library({
			sources: [
				sourceAt(1),
				sourceAt(2),
				sourceAt(3),
				sourceAt(4, { missing_since: '2026-08-17T09:30:00Z' })
			]
		});
		const chips = sourceChips(many, HEALTH_UNREAD);
		expect(chips.shown[0].key).toBe('4');
		expect(chips.shown[0].missing).toBe(true);
		expect(chips.shown.slice(1).every((c) => !c.missing)).toBe(true);
	});

	it('marks a source missing ONLY when missing_since is present', () => {
		// ⚠️ `missing: false` means NOBODY HAS SAID. Nothing sets the column, so it
		// is never a health pass, and no rendering may turn it into one.
		expect(sourceChips(library(), HEALTH_UNREAD).shown[0].missing).toBe(false);
	});

	it('renders no chips at all for a library with zero sources', () => {
		const orphan = library({ sources: [] });
		expect(sourceChips(orphan, HEALTH_UNREAD)).toEqual({ shown: [], more: 0, total: 0 });
	});
});

describe('the State column, and every arm of it is an observation', () => {
	function words(over: Partial<Library>): string[] {
		return libraryStates(library(over), HEALTH_UNREAD).map((m) => m.word);
	}

	it('says NOTHING about a library with nothing to report', () => {
		// ⚠️ THE TEMPTING BUG. `missing_since` is unset on every source of every
		// row today, so an `ok` arm would fire on every library in the product and
		// read as "checked, and fine" while nothing has ever checked anything.
		expect(libraryStates(library(), HEALTH_UNREAD)).toEqual([]);
	});

	/**
	 * ⚠️ THIS TEST CHANGED WITH THE HEALTH JOIN, DELIBERATELY, AND THE CHANGE IS
	 * A WIDENING RATHER THAN A WEAKENING. Two things happened to it:
	 *
	 *   · `err` JOINED THE ACCEPTED SET. The join brought states that ARE errors
	 *     — an instance that is not answering, an instance whose identity is in
	 *     doubt — and `.st--err` is the role the Services screen already paints
	 *     those with. Amber on one screen and red on the other for the same
	 *     `service_instance` row would be two screens disagreeing about one fact.
	 *   · THE SHAPE LIST GREW FROM FIVE TO SIXTEEN, and the new eleven are the
	 *     health-joined ones. A pin over five shapes that predate the join would
	 *     have gone on passing while every new arm went unchecked, which is the
	 *     vacuous-green failure this file exists to prevent.
	 *
	 * WHAT DID NOT CHANGE IS THE INVARIANT: no `ok`, on any shape, under any
	 * read. The join measures a SERVICE and this column describes a LIBRARY, and
	 * the two columns that would let a library be pronounced fine —
	 * `missing_since` and `orphaned_at` — still have no writer (http-api.md §2.4).
	 */
	it('never emits an ok tone at all, under either read', () => {
		const everyShape: Partial<Library>[] = [
			{},
			{ sources: [] },
			{ itemCount: 0 },
			{ enabled: false },
			{ includeInSearch: false },
			{ sources: [source({ missingSince: '2026-08-17T09:30:00Z' })] },
			{ sources: [source({ serviceInstanceId: 99 })] },
			{ sources: [source({ id: 1 }), source({ id: 2, serviceInstanceId: 2 })] }
		];
		const everyRead: HealthRead[] = [
			HEALTH_UNREAD,
			CONNECTED,
			read(healthRow({ state: 'degraded' })),
			read(healthRow({ state: 'down' })),
			read(healthRow({ breakerState: 'open' })),
			read(healthRow({ state: 'needs re-identification' })),
			read(healthRow({ enabled: false })),
			read(healthRow({ stale: true })),
			read(healthRow({ state: 'unknown' })),
			read(healthRow({ lastFullSyncAt: null, workCount: 0 })),
			read(healthRow({ lastFullSyncAt: null, workCount: 12 })),
			read()
		];
		let marks = 0;
		for (const shape of everyShape) {
			for (const r of everyRead) {
				for (const mark of libraryStates(library(shape), r)) {
					marks += 1;
					expect(['err', 'warn', 'none']).toContain(mark.tone);
				}
			}
		}
		// A tone assertion over zero marks is a green that measured nothing.
		expect(marks).toBeGreaterThan(50);
	});

	it('paints no chip with the ok role either, on any read', () => {
		for (const r of [CONNECTED, read(healthRow({ state: 'down' })), HEALTH_UNREAD]) {
			for (const chip of sourceChips(library(), r).shown) {
				expect(['err', 'warn', 'none']).toContain(chip.verdict.tone);
			}
		}
	});

	it('reports a library with no sources, from the ARRAY and not from orphaned_at', () => {
		// `sources: []` is served unconditionally and is an observation;
		// `orphaned_at` has no writer. So the state renders without it.
		const marks = libraryStates(library({ sources: [], itemCount: 0 }), HEALTH_UNREAD);
		expect(marks.map((m) => m.word)).toEqual(['No sources']);
		expect(marks[0].tone).toBe('warn');
		expect(marks[0].at).toBeUndefined();
	});

	it('qualifies that state with orphaned_at the day something writes it', () => {
		const marks = libraryStates(
			library({ sources: [], itemCount: 0, orphanedAt: '2026-08-17T08:00:00Z' }),
			HEALTH_UNREAD
		);
		expect(marks[0].at).toBe('2026-08-17T08:00:00Z');
	});

	it('reports a missing source, with the earliest stamp', () => {
		const base = toLibrarySource(wireSource());
		if (base === undefined) throw new Error('the source fixture must parse');
		const marks = libraryStates(
			library({
				sources: [
					{ ...base, id: 1, missingSince: '2026-08-17T09:30:00Z' },
					{ ...base, id: 2, missingSince: '2026-08-16T04:00:00Z' },
					{ ...base, id: 3 }
				]
			}),
			HEALTH_UNREAD
		);
		expect(marks[0]).toEqual({
			key: 'source-missing',
			word: '2 sources are missing',
			tone: 'warn',
			at: '2026-08-16T04:00:00Z'
		});
	});

	it('counts one missing source in the singular, with no parenthesised plural', () => {
		const base = toLibrarySource(wireSource());
		if (base === undefined) throw new Error('the source fixture must parse');
		const marks = libraryStates(
			library({ sources: [{ ...base, missingSince: '2026-08-17T09:30:00Z' }] }),
			HEALTH_UNREAD
		);
		expect(marks[0].word).toBe('A source is missing');
		expect(marks[0].word).not.toContain('(s)');
	});

	it('reports zero items only where there IS a source to have reported them', () => {
		expect(words({ itemCount: 0 })).toEqual(['No items']);
		// With no sources the row already says the stronger thing, and "No sources"
		// plus "No items" is one fact twice.
		expect(words({ sources: [], itemCount: 0 })).toEqual(['No sources']);
	});

	it('claims nothing about health when it reports zero items', () => {
		// §17.8's own example sentence is "Radarr is connected and reports 0 films",
		// and `connected` is exactly the word nothing on this wire has measured.
		const marks = libraryStates(library({ itemCount: 0 }), HEALTH_UNREAD);
		expect(marks[0].tone).toBe('none');
		expect(marks[0].at).toBeUndefined();
	});

	it('reports the two visibility columns independently, and both at once', () => {
		expect(words({ enabled: false })).toEqual(['Turned off']);
		expect(words({ includeInSearch: false })).toEqual(['Not in search']);
		expect(words({ enabled: false, includeInSearch: false })).toEqual([
			'Turned off',
			'Not in search'
		]);
	});

	it('drops no mark when several are true at once, worst first', () => {
		expect(words({ itemCount: 0, enabled: false, includeInSearch: false })).toEqual([
			'No items',
			'Turned off',
			'Not in search'
		]);
	});

	it('gives every mark a distinct key, so an {#each} can key on it', () => {
		const marks = libraryStates(
			library({ sources: [], enabled: false, includeInSearch: false }),
			HEALTH_UNREAD
		);
		expect(new Set(marks.map((m) => m.key)).size).toBe(marks.length);
	});
});

/**
 * THE HEALTH JOIN — `GET /api/v1/libraries` × `GET /api/v1/services/health`.
 *
 * Every fixture is the shipped wire: `ServiceHealth` as `$lib/api` parses
 * `serviceHealthResponse` (internal/httpapi/services.go:595), and the id both
 * sides key on is `service_instance.id` — `library_source.service_instance_id`
 * REFERENCES it (migration 00005:637, joined at internal/store/libraries.go:344)
 * and the health row's `id` is `si.ID` (internal/httpapi/services.go:714).
 */
describe('one source, joined to the health read', () => {
	function verdictOf(over: Partial<ServiceHealth>) {
		return sourceVerdict(source(), read(healthRow(over)));
	}

	it('says connected only where a probe RAN and passed', () => {
		expect(verdictOf({})).toEqual({ state: 'connected', word: 'connected', tone: 'none' });
	});

	it('never says connected on a healthy row that has never been probed', () => {
		// `stale` outranks `healthy` for the reason $lib/services states: the server
		// answers `healthy` on a stored `last_ok_at` alone, so an instance that
		// answered once when it was added arrives healthy AND stale, and that is a
		// fact about the past rather than a measurement.
		expect(verdictOf({ stale: true })).toEqual({
			state: 'unchecked',
			word: 'not checked',
			tone: 'none'
		});
	});

	it('reads the four unhealthy rows the Services screen reads', () => {
		expect(verdictOf({ state: 'degraded' }).state).toBe('degraded');
		expect(verdictOf({ state: 'down' }).state).toBe('down');
		expect(verdictOf({ breakerState: 'open' }).state).toBe('down');
		expect(verdictOf({ state: 'needs re-identification' })).toEqual({
			state: 'reidentify',
			word: 'may be a different Kavita',
			tone: 'err'
		});
	});

	it('reads a turned-off instance as turned off, not as down', () => {
		// UsArr is not contacting it BY INSTRUCTION. Nothing is broken, so the
		// chip is grey and the row carries no error.
		expect(verdictOf({ enabled: false })).toEqual({
			state: 'off',
			word: 'turned off',
			tone: 'none'
		});
	});

	it('agrees with the Services screen arm for arm, so the two never disagree', () => {
		// The precedence is re-implemented here, and a second implementation of a
		// precedence is the ordinary way two screens come to disagree about one
		// row. This is the pin: `connected` exactly where `stateLabel` says `ok`.
		const now = new Date('2026-08-17T13:00:00Z');
		const rows = [
			healthRow({}),
			healthRow({ stale: true }),
			healthRow({ state: 'degraded' }),
			healthRow({ state: 'down' }),
			healthRow({ breakerState: 'open' }),
			healthRow({ state: 'needs re-identification' }),
			healthRow({ enabled: false }),
			healthRow({ state: 'unknown' })
		];
		for (const row of rows) {
			const mine = sourceVerdict(source(), read(row)).state === 'connected';
			const theirs = stateLabel({ health: row }, now).tone === 'ok';
			expect(mine).toBe(theirs);
		}
	});

	it('reads an instance the health read never mentions as UNLISTED, never as healthy', () => {
		// ABSENCE IS THE POINT. The library names instance 1; the health read
		// answered about instance 7. Nothing has been measured about instance 1.
		const verdict = sourceVerdict(source(), read(healthRow({ id: 7 })));
		expect(verdict).toEqual({ state: 'unlisted', word: 'not listed', tone: 'none' });
	});

	it('reads an unread health endpoint as unread on every source', () => {
		expect(sourceVerdict(source(), HEALTH_UNREAD)).toEqual({
			state: 'unread',
			word: '',
			tone: 'none'
		});
	});
});

describe('the Sources cell carries the word, never the colour alone', () => {
	it('prints the health word inside every chip once health has been read', () => {
		const chips = sourceChips(library(), CONNECTED);
		expect(chips.shown[0].word).toBe('connected');
		expect(chips.shown[0].title).toBe('Kavita Manga · Books · connected');
	});

	it('prints NO health word at all while health is unread', () => {
		// A bare chip must not mean `connected`, so on the read that measured
		// nothing every chip is bare and the sentence above the table says why.
		const chips = sourceChips(library(), HEALTH_UNREAD);
		expect(chips.shown[0].word).toBe('');
		expect(chips.shown[0].title).toBe('Kavita Manga · Books');
	});

	it('keeps the container name and the health word apart on the title', () => {
		const chips = sourceChips(library(), read(healthRow({ state: 'down' })));
		expect(chips.shown[0].title).toBe('Kavita Manga · Books · not answering');
		expect(chips.shown[0].verdict.tone).toBe('err');
	});

	it('says so on the title when the container itself stopped being reported', () => {
		const one = library({ sources: [source({ missingSince: '2026-08-17T09:30:00Z' })] });
		expect(sourceChips(one, CONNECTED).shown[0].title).toBe(
			'Kavita Manga · Books · connected · no longer reported'
		);
	});

	it('prints the word the COLOUR is about, so the two never disagree', () => {
		// Measured in Chromium before this rule existed: a source with
		// `missing_since` set on a healthy instance rendered amber and read
		// `connected`, which paints one fact and names another.
		const one = library({ sources: [source({ missingSince: '2026-08-17T09:30:00Z' })] });
		expect(sourceChips(one, CONNECTED).shown[0].word).toBe('no longer reported');
		expect(sourceChips(one, CONNECTED).shown[0].verdict.state).toBe('connected');
	});

	it('lets a real problem keep the word when the container is missing too', () => {
		const one = library({ sources: [source({ missingSince: '2026-08-17T09:30:00Z' })] });
		const chip = sourceChips(one, read(healthRow({ state: 'down' }))).shown[0];
		expect(chip.word).toBe('not answering');
		expect(chip.title).toBe('Kavita Manga · Books · not answering · no longer reported');
	});

	it('still prints the missing word when health was never read', () => {
		const one = library({ sources: [source({ missingSince: '2026-08-17T09:30:00Z' })] });
		expect(sourceChips(one, HEALTH_UNREAD).shown[0].word).toBe('no longer reported');
	});

	it('hoists the worst source in front of the cap, worst first', () => {
		const sources = [
			source({ id: 1, serviceInstanceId: 1 }),
			source({ id: 2, serviceInstanceId: 2 }),
			source({ id: 3, serviceInstanceId: 3 }),
			source({ id: 4, serviceInstanceId: 4 })
		];
		const chips = sourceChips(
			library({ sources }),
			read(
				healthRow({ id: 1 }),
				healthRow({ id: 2, state: 'degraded' }),
				healthRow({ id: 3, state: 'down' })
				// instance 4 is absent from the read: unlisted, which outranks a
				// measurement that passed and is outranked by both problems.
			)
		);
		expect(chips.shown.map((c) => c.key)).toEqual(['3', '2', '4']);
		expect(chips.more).toBe(1);
	});
});

describe('§17.8s row states, now that health supplies the half libraries never carried', () => {
	function marksOf(over: Partial<Library>, r: HealthRead) {
		return libraryStates(library(over), r);
	}
	function twoSources(): LibrarySource[] {
		return [source({ id: 1, serviceInstanceId: 1 }), source({ id: 2, serviceInstanceId: 2 })];
	}

	it('reports ONE SOURCE DEGRADED and names it', () => {
		// §17.8: *"one source degraded (non-modal banner naming it; the grid does
		// not grey out)"*. The name is the detail; the grid is untouched.
		const marks = marksOf(
			{ sources: twoSources() },
			read(healthRow({ id: 1 }), healthRow({ id: 2, state: 'degraded', name: 'Kavita Books' }))
		);
		expect(marks[0]).toEqual({
			key: 'source-degraded',
			word: 'A source is failing some attempts',
			tone: 'warn',
			detail: 'Kavita Manga'
		});
	});

	it('reports ALL SOURCES DOWN as one mark, not as one mark per source', () => {
		// §17.8 calls this state the replica principle's demo: everything is
		// unreachable and the table is still here, counts and all.
		const marks = marksOf(
			{ sources: twoSources() },
			read(healthRow({ id: 1, state: 'down' }), healthRow({ id: 2, breakerState: 'open' }))
		);
		expect(marks.map((m) => m.word)).toEqual(['No source is answering']);
		expect(marks[0].tone).toBe('err');
		expect(marks[0].detail).toBe('this list still renders from the replica');
	});

	it('separates one source down from every source down', () => {
		const marks = marksOf(
			{ sources: twoSources() },
			read(healthRow({ id: 1 }), healthRow({ id: 2, state: 'down' }))
		);
		expect(marks[0].word).toBe('A source is not answering');
		expect(marks[0].key).toBe('source-down');
	});

	it('reports NEEDS RE-IDENTIFICATION, and says that sync is paused', () => {
		// §17.8: *"blocking banner, membership recompute paused, because membership
		// derived from an untrustworthy id space is worse than stale membership"*.
		// The detail is $lib/services own words, so both screens say one thing.
		const marks = marksOf({}, read(healthRow({ state: 'needs re-identification' })));
		expect(marks[0]).toEqual({
			key: 'reidentify',
			word: 'A source may be a different Kavita',
			tone: 'err',
			detail: 'sync is paused'
		});
	});

	it('reports SOURCES HEALTHY WITH ZERO ITEMS, which is the state the join unlocked', () => {
		// §17.8's own example is *"Radarr is connected and reports 0 films"*, and
		// `connected` is exactly what GET /api/v1/libraries never measured.
		const marks = marksOf({ itemCount: 0 }, CONNECTED);
		expect(marks).toEqual([
			{
				key: 'connected-empty',
				word: 'Connected and empty',
				tone: 'none',
				detail: 'Kavita Manga reports no items'
			}
		]);
	});

	it('keeps the bare No items answer for a zero count nothing has explained', () => {
		// The pre-join rendering survives for exactly the case it was written for.
		expect(marksOf({ itemCount: 0 }, HEALTH_UNREAD).map((m) => m.word)).toEqual(['No items']);
		expect(marksOf({ itemCount: 0 }, read(healthRow({ id: 7 }))).map((m) => m.word)).toEqual([
			'A source is not in Services',
			'No items'
		]);
	});

	it('tells NEVER SYNCED apart from connected and empty, which §17.8 insists on', () => {
		// http-api.md §3.2: `null` + `0` is "never synced" and a timestamp + `0` is
		// "synced, and found nothing". §17.8 wants the second said as *"Radarr is
		// connected and reports 0 films"* and the first NOT said that way.
		const marks = marksOf(
			{ itemCount: 0 },
			read(healthRow({ lastFullSyncAt: null, workCount: 0 }))
		);
		expect(marks.map((m) => m.word)).toEqual(['A source has never synced']);
		expect(marks[0].detail).toBe('Kavita Manga');
	});

	it('reports an import that did not finish, and does not call it importing', () => {
		// `null` + `>0` is http-api.md §3.2's partial import. It is NOT §17.8's
		// *importing* state: nothing reports an import in flight, and this pair is
		// equally the aftermath of one that failed.
		const marks = marksOf(
			{ itemCount: 4 },
			read(healthRow({ lastFullSyncAt: null, workCount: 12 }))
		);
		expect(marks[0].key).toBe('partial-import');
		expect(marks[0].word).toBe('An import did not finish');
		expect(marks[0].detail).toBe('this count may be short');
		expect(marks.map((m) => m.word).join(' ')).not.toContain('mporting');
	});

	it('says nothing about a catalogue an indexer does not have', () => {
		// A Prowlarr reports `null` / `0` exactly like an unsynced catalogue source
		// and only `role` separates them (http-api.md §3.2). §17.8 proposes no
		// library for one, so this is the guard for the day something changes.
		const marks = marksOf(
			{ itemCount: 0 },
			read(healthRow({ role: 'indexer', kind: 'prowlarr', lastFullSyncAt: null, workCount: 0 }))
		);
		expect(marks.map((m) => m.word)).toEqual(['Connected and empty']);
	});

	it('orders the marks worst first, and drops none of them', () => {
		const marks = marksOf(
			{
				sources: twoSources(),
				itemCount: 0,
				enabled: false,
				includeInSearch: false
			},
			read(
				healthRow({ id: 1, state: 'needs re-identification' }),
				healthRow({ id: 2, state: 'down' })
			)
		);
		// `No items` survives beside the two health marks rather than being
		// suppressed by them: the count really is zero, and a source that is not
		// answering may hold plenty, so the two are different facts and neither
		// explains the other.
		expect(marks.map((m) => m.word)).toEqual([
			'A source may be a different Kavita',
			'A source is not answering',
			'No items',
			'Turned off',
			'Not in search'
		]);
		expect(new Set(marks.map((m) => m.key)).size).toBe(marks.length);
	});

	it('renders NOTHING health-derived while health is unread', () => {
		// THE REGRESSION THAT MATTERS: health fails, libraries succeeds, and the
		// table draws exactly what it drew before the join existed.
		const sources = twoSources();
		expect(libraryStates(library({ sources, itemCount: 0 }), HEALTH_UNREAD)).toEqual([
			{ key: 'no-items', word: 'No items', tone: 'none' }
		]);
	});
});

describe('the two reads can disagree, in both directions', () => {
	it('reports a source whose instance the health read does not list', () => {
		const marks = libraryStates(library(), read(healthRow({ id: 7, name: 'Kavita Books' })));
		expect(marks[0]).toEqual({
			key: 'source-unlisted',
			word: 'A source is not in Services',
			tone: 'none',
			detail: 'Kavita Manga'
		});
	});

	it('reports a health row no library names, as a sentence rather than as a row', () => {
		const orphanService = healthRow({ id: 7, name: 'Kavita Books' });
		const unbound = unboundServices([library()], read(healthRow({ id: 1 }), orphanService));
		expect(unbound.map((h) => h.id)).toEqual([7]);
		expect(unboundNote(unbound)).toBe('Kavita Books feeds no library yet.');
	});

	it('counts several unbound services rather than naming them all', () => {
		const unbound = unboundServices(
			[library()],
			read(healthRow({ id: 7 }), healthRow({ id: 8 }), healthRow({ id: 1 }))
		);
		expect(unboundNote(unbound)).toBe('2 services feed no library yet.');
	});

	it('never counts an indexer as unbound, because §17.8 proposes it no library', () => {
		const unbound = unboundServices(
			[library()],
			read(healthRow({ id: 1 }), healthRow({ id: 9, kind: 'prowlarr', role: 'indexer' }))
		);
		expect(unbound).toEqual([]);
		expect(unboundNote(unbound)).toBe('');
	});

	it('claims no unbound service at all while health is unread', () => {
		expect(unboundServices([library()], HEALTH_UNREAD)).toEqual([]);
	});
});

describe('the sentence above the table says which read is missing', () => {
	it('names health, not the libraries, when health is what failed', () => {
		const note = healthNote(HEALTH_UNREAD);
		expect(note).toContain('Service health could not be read');
		expect(note).toContain('are complete');
		// It must not send the reader after the table, which is intact.
		expect(note).not.toContain('Try again');
	});

	it('says what an absent source means once health HAS been read', () => {
		expect(healthNote(CONNECTED)).toContain('rather than being counted as connected');
	});
});

describe('the three ways this screen shows nothing read differently', () => {
	it('reads a 401 as an ended session, with no verbatim block', () => {
		// §2.6 gives the endpoint exactly two error statuses. This one is a PROMPT:
		// nothing is broken, nothing was lost, and the server did not fail, so
		// there is no upstream text to quote.
		const failure = describeFailure(
			new ApiError(
				'HTTP 401',
				401,
				LIBRARIES_URL,
				'unauthorized',
				'',
				'this request has no session'
			)
		);
		expect(failure.k).toBe('session');
		expect(Object.hasOwn(failure, 'verbatim')).toBe(false);
		expect(failure.title).toBe('Your session has ended');
	});

	it('reads the code as well as the status, never the prose', () => {
		const failure = describeFailure(new ApiError('nope', 0, LIBRARIES_URL, 'unauthorized'));
		expect(failure.k).toBe('session');
	});

	it('reads a 500 as a failed read, and quotes the server verbatim', () => {
		const failure = describeFailure(
			new ApiError(
				'HTTP 500',
				500,
				LIBRARIES_URL,
				'internal',
				'',
				'your libraries could not be read'
			)
		);
		expect(failure.k).toBe('failed');
		expect(failure).toHaveProperty('verbatim', 'your libraries could not be read');
	});

	it('reads a transport failure as a failed read too', () => {
		// $lib/api wraps a fetch rejection as an ApiError with status 0.
		const failure = describeFailure(new ApiError('TypeError: Failed to fetch', 0, LIBRARIES_URL));
		expect(failure.k).toBe('failed');
		expect(failure).toHaveProperty('verbatim', 'TypeError: Failed to fetch');
	});

	it('reads a thrown non-error as a failed read without losing it', () => {
		expect(describeFailure('kaboom')).toHaveProperty('verbatim', 'kaboom');
	});

	it('never sends a failed local read to Services', () => {
		// The endpoint is two SQLite statements. A service being down cannot cause
		// this, and a fix that cannot work is worse than none.
		const failure = describeFailure(new ApiError('HTTP 500', 500, LIBRARIES_URL, 'internal'));
		expect(failure.text.toLowerCase()).not.toContain('services');
	});

	it('says nothing about a failure being the same as an empty install', () => {
		// The zero state is deliberately NOT in this union: it is a successful read
		// of nothing, and it is the list's own `empty` state.
		expect(describeFailure(new ApiError('x', 500, LIBRARIES_URL)).k).not.toBe('empty');
	});
});
