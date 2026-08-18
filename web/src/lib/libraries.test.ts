import { describe, expect, it } from 'vitest';
import { ApiError } from './api';
import {
	LIBRARIES_URL,
	describeFailure,
	itemCountText,
	kindLabel,
	libraryStates,
	sourceChips,
	toFormats,
	toLibraries,
	toLibrary,
	toLibrarySource,
	type Library
} from './libraries';

/**
 * §17.8's ROW VIEW, PINNED — over the wire `internal/httpapi/libraries.go`
 * actually marshals and the contract `docs/reference/http-api.md` §2 writes
 * down.
 *
 * Every fixture below is shaped from the server's own source rather than from a
 * summary of it: `libraryResponse` and `librarySourceResponse` for the two
 * structs, §2.2's worked example for the optional keys, and §2.4 for the three
 * fields that describe states nothing in the tree can currently reach.
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
		const chips = sourceChips(library());
		expect(chips.total).toBe(1);
		expect(chips.more).toBe(0);
		expect(chips.shown[0].label).toBe('Kavita Manga');
		expect(chips.shown[0].serviceInstanceId).toBe(1);
	});

	it("puts the upstream's own container name on the title, not in the label", () => {
		// §17.8 wants the upstream's own name for the container; the row view has
		// no second line for it, so it rides the `title` the truncation already
		// needs (§9.1 tier 1).
		expect(sourceChips(library()).shown[0].title).toBe('Kavita Manga · Books');
	});

	it('falls back to a title of just the label when no container name travelled', () => {
		const one = library({ sources: [sourceAt(2, { container_name: undefined })] });
		expect(sourceChips(one).shown[0].title).toBe('Kavita Manga');
	});

	it('names a source whose service name did not travel rather than dropping it', () => {
		const one = library({ sources: [sourceAt(2, { service_name: undefined })] });
		expect(sourceChips(one).shown[0].label).toBe('Unnamed service');
	});

	it('caps at three plus a count, per §9.1', () => {
		const many = library({ sources: [1, 2, 3, 4, 5].map((n) => sourceAt(n)) });
		const chips = sourceChips(many);
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
		const chips = sourceChips(many);
		expect(chips.shown[0].key).toBe('4');
		expect(chips.shown[0].missing).toBe(true);
		expect(chips.shown.slice(1).every((c) => !c.missing)).toBe(true);
	});

	it('marks a source missing ONLY when missing_since is present', () => {
		// ⚠️ `missing: false` means NOBODY HAS SAID. Nothing sets the column, so it
		// is never a health pass, and no rendering may turn it into one.
		expect(sourceChips(library()).shown[0].missing).toBe(false);
	});

	it('renders no chips at all for a library with zero sources', () => {
		const orphan = library({ sources: [] });
		expect(sourceChips(orphan)).toEqual({ shown: [], more: 0, total: 0 });
	});
});

describe('the State column, and every arm of it is an observation', () => {
	function words(over: Partial<Library>): string[] {
		return libraryStates(library(over)).map((m) => m.word);
	}

	it('says NOTHING about a library with nothing to report', () => {
		// ⚠️ THE TEMPTING BUG. `missing_since` is unset on every source of every
		// row today, so an `ok` arm would fire on every library in the product and
		// read as "checked, and fine" while nothing has ever checked anything.
		expect(libraryStates(library())).toEqual([]);
	});

	it('never emits an ok tone at all', () => {
		const everyShape: Partial<Library>[] = [
			{},
			{ sources: [] },
			{ itemCount: 0 },
			{ enabled: false },
			{ includeInSearch: false }
		];
		for (const shape of everyShape) {
			for (const mark of libraryStates(library(shape))) {
				expect(['warn', 'none']).toContain(mark.tone);
			}
		}
	});

	it('reports a library with no sources, from the ARRAY and not from orphaned_at', () => {
		// `sources: []` is served unconditionally and is an observation;
		// `orphaned_at` has no writer. So the state renders without it.
		const marks = libraryStates(library({ sources: [], itemCount: 0 }));
		expect(marks.map((m) => m.word)).toEqual(['No sources']);
		expect(marks[0].tone).toBe('warn');
		expect(marks[0].at).toBeUndefined();
	});

	it('qualifies that state with orphaned_at the day something writes it', () => {
		const marks = libraryStates(
			library({ sources: [], itemCount: 0, orphanedAt: '2026-08-17T08:00:00Z' })
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
			})
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
			library({ sources: [{ ...base, missingSince: '2026-08-17T09:30:00Z' }] })
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
		const marks = libraryStates(library({ itemCount: 0 }));
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
		const marks = libraryStates(library({ sources: [], enabled: false, includeInSearch: false }));
		expect(new Set(marks.map((m) => m.key)).size).toBe(marks.length);
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
