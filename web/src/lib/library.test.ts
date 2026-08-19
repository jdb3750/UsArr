import { describe, expect, it } from 'vitest';
import {
	EMPTY_RECENT_FEED,
	LIBRARY_RECENT_URL,
	MEDIA_TYPES,
	appendPage,
	availabilityMark,
	hasMore,
	haveCell,
	isMediaType,
	mediaTypeLabel,
	nextRequest,
	recentRequestUrl,
	toAvailability,
	posterUrl,
	toRecentItem,
	toRecentPage,
	IMAGE_WIDTHS,
	type RecentFeed,
	type RecentItem
} from './library';

/**
 * BLOCK C's CLIENT RULES, PINNED. ARCHITECTURE.md §17.2 as amended by ADR-0028,
 * §6.3's render rule, and `docs/reference/schema.md`'s availability blob.
 *
 * Every fixture below is shaped from the SERVER's own source rather than from a
 * summary of it: `recentWorkResponse` in `internal/httpapi/library.go` for the
 * row, `EncodeRecentWorksCursor` in `internal/store/recent.go` for the cursor,
 * and schema.md's three worked examples for the blob.
 */

/** One row as `internal/httpapi/library.go` marshals it. */
function wireRow(over: Record<string, unknown> = {}): Record<string, unknown> {
	return {
		id: 12,
		media_type: 'comics',
		kind: 'comic',
		title: 'Berserk',
		year: 1989,
		added_at: '2026-08-10T10:00:00Z',
		have_count: 43,
		want_count: 17,
		poster_key: 'aaaaaaaaaaaaaaaa',
		...over
	};
}

describe('the media-type vocabulary', () => {
	it('is exactly the six §17.2 closes the navigation enum at', () => {
		// The same six strings internal/store/recent.go declares as
		// MediaTypeMovies … MediaTypeComics.
		expect([...MEDIA_TYPES]).toEqual(['movies', 'tv', 'music', 'ebooks', 'audiobooks', 'comics']);
	});

	it('labels every one of them', () => {
		for (const t of MEDIA_TYPES) {
			expect(isMediaType(t)).toBe(true);
			expect(mediaTypeLabel(t)).not.toBe('');
		}
		expect(mediaTypeLabel('audiobooks')).toBe('Audiobooks');
		expect(mediaTypeLabel('ebooks')).toBe('Ebooks');
	});

	it('renders a value outside the six verbatim rather than hiding it', () => {
		expect(isMediaType('games')).toBe(false);
		expect(mediaTypeLabel('games')).toBe('games');
	});

	it('takes the Type cell from media_type and never from kind', () => {
		// THE RULE THIS TEST EXISTS FOR. Both of these are kind 'book'; only
		// media_type separates them, because edition.format is not on this wire
		// (§17.2: "the Ebooks/Audiobooks split is server-side only in v0.1").
		const ebook = toRecentItem(wireRow({ media_type: 'ebooks', kind: 'book' }));
		const audiobook = toRecentItem(wireRow({ media_type: 'audiobooks', kind: 'book' }));
		expect(ebook?.kind).toBe('book');
		expect(audiobook?.kind).toBe('book');
		expect(mediaTypeLabel(ebook?.mediaType ?? '')).toBe('Ebooks');
		expect(mediaTypeLabel(audiobook?.mediaType ?? '')).toBe('Audiobooks');
	});
});

describe('a row off the wire', () => {
	it('carries every field the allowlist publishes', () => {
		const item = toRecentItem(wireRow());
		expect(item).toEqual({
			id: 12,
			mediaType: 'comics',
			kind: 'comic',
			title: 'Berserk',
			year: 1989,
			addedAt: '2026-08-10T10:00:00Z',
			haveCount: 43,
			wantCount: 17,
			posterKey: 'aaaaaaaaaaaaaaaa'
		});
	});

	it('leaves an absent year absent rather than rendering 0', () => {
		const item = toRecentItem(wireRow({ year: undefined }));
		expect(item).toBeDefined();
		expect('year' in (item as RecentItem)).toBe(false);
		expect(item?.year).toBeUndefined();
	});

	it('leaves an absent added_at absent rather than rendering the epoch', () => {
		const item = toRecentItem(wireRow({ added_at: undefined }));
		expect(item).toBeDefined();
		expect('addedAt' in (item as RecentItem)).toBe(false);
	});

	it('never turns a null year into a number', () => {
		// The server omits the key; a proxy that rewrote it to null must not
		// become 0 either.
		expect(toRecentItem(wireRow({ year: null }))?.year).toBeUndefined();
	});

	it('keeps a row whose title is empty, because dropping it would shorten the table', () => {
		const item = toRecentItem(wireRow({ title: '' }));
		expect(item?.id).toBe(12);
		expect(item?.title).toBe('');
	});

	it('drops a frame with no id, which is the row key and the item route', () => {
		expect(toRecentItem(wireRow({ id: undefined }))).toBeUndefined();
		expect(toRecentItem(wireRow({ id: 'twelve' }))).toBeUndefined();
		expect(toRecentItem(null)).toBeUndefined();
	});
});

describe('the availability blob', () => {
	it('parses schema.md’s tier example and keys each tier by its own name', () => {
		const a = toAvailability({
			k: 'tier',
			'1080p': { have: 250, total: 300 },
			'2160p': { have: 40, total: 300 }
		});
		expect(a?.k).toBe('tier');
		if (a?.k !== 'tier') throw new Error('expected a tier blob');
		expect(a.buckets).toEqual([
			{ key: '1080p', label: '1080p', have: 250, total: 300 },
			{ key: '2160p', label: '2160p', have: 40, total: 300 }
		]);
	});

	it('parses schema.md’s edition example and takes the label from the blob', () => {
		const a = toAvailability({
			k: 'edition',
			'edition:mbz_release:def-456': { have: 0, total: 22, label: '2017 remaster (2CD)' },
			'edition:mbz_release:abc-123': { have: 12, total: 12, label: '2000 original' }
		});
		if (a?.k !== 'edition') throw new Error('expected an edition blob');
		expect(a.buckets.map((b) => b.label)).toEqual(['2017 remaster (2CD)', '2000 original']);
		// The opaque key never becomes the label, because an edition id is not
		// something a user reads.
		expect(a.buckets[0].key).toBe('edition:mbz_release:def-456');
	});

	it('parses both of schema.md’s count examples, keeping a null total null', () => {
		const declared = toAvailability({
			k: 'count',
			have: 43,
			total: 60,
			total_source: 'komga:totalBookCount',
			missing: ['7', '12', '30-32']
		});
		expect(declared).toEqual({
			k: 'count',
			have: 43,
			total: 60,
			totalSource: 'komga:totalBookCount',
			missing: ['7', '12', '30-32']
		});

		const undeclared = toAvailability({
			k: 'count',
			have: 43,
			total: null,
			total_source: null,
			missing: ['7', '12']
		});
		if (undeclared?.k !== 'count') throw new Error('expected a count blob');
		// ⚠️ NOT 0. schema.md: "`total: null` is not `total: 0`".
		expect(undeclared.total).toBeNull();
		expect(undeclared.total).not.toBe(0);
	});

	it('answers undefined for an unrecognised discriminator rather than guessing', () => {
		expect(toAvailability({ k: 'runtime', have: 1, total: 1 })).toBeUndefined();
		// `k` is required on every non-null blob (schema.md), so a blob without
		// one is not a shape this renderer may switch on.
		expect(toAvailability({ '1080p': { have: 1, total: 1 } })).toBeUndefined();
		expect(toAvailability(undefined)).toBeUndefined();
		expect(toAvailability('{}')).toBeUndefined();
	});
});

describe('§6.3’s render rule', () => {
	it('ticks only a complete rollup over a positive total', () => {
		expect(availabilityMark(300, 300)).toEqual({ k: 'complete' });
		expect(availabilityMark(1, 1)).toEqual({ k: 'complete' });
	});

	it('crosses a rollup that holds nothing', () => {
		expect(availabilityMark(0, 300)).toEqual({ k: 'none' });
		expect(availabilityMark(0, 0)).toEqual({ k: 'none' });
		expect(availabilityMark(0, null)).toEqual({ k: 'none' });
	});

	it('renders the fraction in between', () => {
		expect(availabilityMark(250, 300)).toEqual({ k: 'fraction', have: 250, total: 300 });
	});

	/**
	 * ⚠️ THE CASE THE WHOLE POLYMORPHIC SHAPE EXISTS FOR. schema.md: "§6.3's
	 * render rule (`have == total && total > 0` → ✓) must never fire" on a null
	 * total. `null == 0` is true in a loose comparison and `have === total` with
	 * a null total is false only by accident of types, so this is asserted
	 * rather than assumed.
	 */
	it('NEVER ticks a count whose total is null, at any have', () => {
		expect(availabilityMark(43, null)).toEqual({ k: 'partial', have: 43 });
		expect(availabilityMark(1, null)).toEqual({ k: 'partial', have: 1 });
		for (const have of [0, 1, 43, 9999]) {
			expect(availabilityMark(have, null).k).not.toBe('complete');
		}
	});

	it('does not tick a total of zero either, however the have compares', () => {
		// `total: 0` means "the series is empty", and `have == total` is true of
		// it — which is why §6.3's rule carries `&& total > 0` and this carries it
		// too.
		expect(availabilityMark(0, 0).k).not.toBe('complete');
	});
});

describe('the Have cell', () => {
	it('renders a comic with no declared total as a bare count and its gaps', () => {
		const item = toRecentItem(
			wireRow({
				availability: {
					k: 'count',
					have: 43,
					total: null,
					total_source: null,
					missing: ['7', '12']
				}
			})
		) as RecentItem;
		const cell = haveCell(item);
		expect(cell.lines).toEqual([{ key: 'count', label: '', mark: { k: 'partial', have: 43 } }]);
		expect(cell.gaps).toBe('7, 12');
		// §17.2's `· N missing`, from the row's own want_count.
		expect(cell.missing).toBe('17 missing');
	});

	it('ticks a comic once a total is declared and met', () => {
		const item = toRecentItem(
			wireRow({
				want_count: 0,
				availability: {
					k: 'count',
					have: 60,
					total: 60,
					total_source: 'komga:totalBookCount',
					missing: []
				}
			})
		) as RecentItem;
		const cell = haveCell(item);
		expect(cell.lines[0].mark).toEqual({ k: 'complete' });
		expect(cell.missing).toBe('');
	});

	it('renders one line per tier, labelled', () => {
		const item = toRecentItem(
			wireRow({
				media_type: 'tv',
				kind: 'series',
				availability: {
					k: 'tier',
					'1080p': { have: 250, total: 300 },
					'2160p': { have: 40, total: 300 }
				}
			})
		) as RecentItem;
		const cell = haveCell(item);
		expect(cell.lines).toEqual([
			{ key: '1080p', label: '1080p', mark: { k: 'fraction', have: 250, total: 300 } },
			{ key: '2160p', label: '2160p', mark: { k: 'fraction', have: 40, total: 300 } }
		]);
		expect(cell.more).toBe(0);
	});

	it('caps the bucket list at three and counts the rest', () => {
		const item = toRecentItem(
			wireRow({
				media_type: 'music',
				kind: 'album',
				availability: {
					k: 'edition',
					a: { have: 1, total: 2, label: 'A' },
					b: { have: 1, total: 2, label: 'B' },
					c: { have: 1, total: 2, label: 'C' },
					d: { have: 1, total: 2, label: 'D' },
					e: { have: 1, total: 2, label: 'E' }
				}
			})
		) as RecentItem;
		const cell = haveCell(item);
		expect(cell.lines).toHaveLength(3);
		expect(cell.more).toBe(2);
	});

	/**
	 * ⚠️ THE INVENTED-DENOMINATOR BUG, PINNED. `have_count + want_count` looks
	 * like a free total, and on every row whose want_count is 0 it would render
	 * a tick — "you have all of it" — out of two numbers that never said so.
	 */
	it('never ticks a row that has no availability blob, even at want_count 0', () => {
		const item = toRecentItem(wireRow({ have_count: 43, want_count: 0 })) as RecentItem;
		expect(item.availability).toBeUndefined();
		const cell = haveCell(item);
		expect(cell.lines[0].mark).toEqual({ k: 'uncounted' });
		expect(cell.lines[0].mark.k).not.toBe('complete');
		expect(cell.missing).toBe('');
	});

	/**
	 * ⚠️ THE INVENTED-ZERO BUG, PINNED — AND IT SHIPPED. Every row in the owner's
	 * library rendered ✗ *none held*, because the absent-blob branch handed
	 * `have_count` to `availabilityMark` and `have_count` is `NOT NULL DEFAULT 0`,
	 * so a work nobody has counted arrived at `have === 0` → `none`.
	 *
	 * `docs/reference/http-api.md` §1.4.1 is the contract: the absence of
	 * `availability` *"means no count has ever been computed for that work"*, and
	 * a consumer *"must not render an absent blob as `0`, as \"none\", or as any
	 * glyph, bar or accessible name that asserts emptiness"*.
	 */
	it('says NOT COUNTED for an absent blob, never none, at any have_count', () => {
		for (const haveCount of [0, 1, 43, 9999]) {
			const item = toRecentItem(wireRow({ have_count: haveCount })) as RecentItem;
			expect(item.availability).toBeUndefined();
			const cell = haveCell(item);
			expect(cell.lines).toEqual([{ key: 'have', label: '', mark: { k: 'uncounted' } }]);
			expect(cell.lines[0].mark.k).not.toBe('none');
			expect(cell.lines[0].mark.k).not.toBe('complete');
		}
	});

	/**
	 * ⚠️ THE SELF-CORRECTION, PROVEN RATHER THAN ASSERTED. §1.4.1: *"the first
	 * flush that has anything to count publishes the key — including when the
	 * honest answer is `have: 0` — and the row moves from *not counted* to counted
	 * with no wire change behind it"*. So the SAME code must render a real ✗ the
	 * day the rollup ships, with no second edit: the two rows below differ only in
	 * whether the key is on the wire, and they must not render the same mark.
	 */
	it('renders a PRESENT blob carrying have 0 as a real none, not as uncounted', () => {
		const counted = toRecentItem(
			wireRow({
				have_count: 0,
				want_count: 0,
				availability: { k: 'count', have: 0, total: null, total_source: null, missing: [] }
			})
		) as RecentItem;
		expect(counted.availability).toBeDefined();
		expect(haveCell(counted).lines[0].mark).toEqual({ k: 'none' });

		const uncounted = toRecentItem(wireRow({ have_count: 0, want_count: 0 })) as RecentItem;
		expect(haveCell(uncounted).lines[0].mark).toEqual({ k: 'uncounted' });

		// The same `have_count: 0` on both. It is the KEY that separates them, and
		// §1.4.1 says so: `have_count: 0` "is not evidence of anything on its own".
		expect(counted.haveCount).toBe(uncounted.haveCount);
		expect(haveCell(counted).lines[0].mark).not.toEqual(haveCell(uncounted).lines[0].mark);
	});

	/**
	 * A present blob with a real partial count is untouched by any of this: it
	 * has been counted, and what it renders is what it always rendered.
	 */
	it('leaves a present partial count exactly as it was', () => {
		const item = toRecentItem(
			wireRow({
				media_type: 'tv',
				kind: 'series',
				availability: { k: 'tier', '1080p': { have: 250, total: 300 } }
			})
		) as RecentItem;
		expect(haveCell(item).lines[0].mark).toEqual({ k: 'fraction', have: 250, total: 300 });
	});
});

describe('the page request', () => {
	it('asks for the newest items with no cursor parameter at all', () => {
		expect(recentRequestUrl({ limit: 200 })).toBe(`${LIBRARY_RECENT_URL}?limit=200`);
	});

	it('percent-encodes the cursor rather than concatenating it', () => {
		// EncodeRecentWorksCursor base64url-encodes a payload embedding SQLite's
		// own datetime layout, which carries a literal space; `+` and `/` are not
		// in base64url but a query string still has to be built, not glued.
		const url = recentRequestUrl({ limit: 200, cursor: 'MWQxMi4yMDI2-_a b' });
		expect(url.startsWith(`${LIBRARY_RECENT_URL}?limit=200&cursor=`)).toBe(true);
		expect(url).not.toContain(' ');
		expect(new URL(url, 'http://x').searchParams.get('cursor')).toBe('MWQxMi4yMDI2-_a b');
	});

	it('reads a page, echoing the limit the server applied', () => {
		const page = toRecentPage({ items: [wireRow()], limit: 200, next_cursor: 'abc' }, 10_000);
		expect(page.items).toHaveLength(1);
		// The server clamps to RecentWorksMaxLimit; the echo is what says so.
		expect(page.limit).toBe(200);
		expect(page.nextCursor).toBe('abc');
	});

	it('leaves next_cursor absent rather than empty on the last page', () => {
		const page = toRecentPage({ items: [], limit: 50 }, 50);
		expect(page.nextCursor).toBeUndefined();
		expect('nextCursor' in page).toBe(false);
	});
});

describe('the paging stop rule', () => {
	const page = (items: RecentItem[], nextCursor?: string) => {
		const p = { items, limit: 200 };
		return nextCursor === undefined ? p : { ...p, nextCursor };
	};
	const row = (id: number) => toRecentItem(wireRow({ id })) as RecentItem;

	it('asks for the first page with no cursor when nothing has been read', () => {
		expect(nextRequest(EMPTY_RECENT_FEED, 200)).toEqual({ limit: 200 });
		expect(hasMore(EMPTY_RECENT_FEED)).toBe(false);
	});

	it('carries the server’s cursor and the SAME limit into the next request', () => {
		const feed = appendPage(EMPTY_RECENT_FEED, page([row(1)], 'MWQxMi4yMDI2'));
		expect(nextRequest(feed, 200)).toEqual({ limit: 200, cursor: 'MWQxMi4yMDI2' });
	});

	it('stops only when next_cursor is ABSENT', () => {
		let feed: RecentFeed = appendPage(EMPTY_RECENT_FEED, page([row(1)], 'c1'));
		expect(hasMore(feed)).toBe(true);
		feed = appendPage(feed, page([row(2)]));
		expect(hasMore(feed)).toBe(false);
		expect(nextRequest(feed, 200)).toBeUndefined();
	});

	/**
	 * ⚠️ THE BUG THIS RULE COMES FROM. `ListRecentWorks` walks the dated range
	 * and hands off to a second statement for the undated tail, so a SHORT page
	 * is legitimate mid-sequence. A client that stopped on
	 * `items.length < limit` would truncate the table at exactly the row whose
	 * upstream reported no creation date.
	 */
	it('does not stop on a short page that still carries a cursor', () => {
		const shortPage = page([row(1), row(2)], 'still-more');
		const feed = appendPage(EMPTY_RECENT_FEED, shortPage);
		expect(feed.items.length).toBeLessThan(200);
		expect(hasMore(feed)).toBe(true);
		expect(nextRequest(feed, 200)).toEqual({ limit: 200, cursor: 'still-more' });
	});

	it('does not stop on an EMPTY page that still carries a cursor', () => {
		const feed = appendPage(EMPTY_RECENT_FEED, page([], 'still-more'));
		expect(feed.items).toHaveLength(0);
		expect(feed.loaded).toBe(true);
		expect(hasMore(feed)).toBe(true);
	});

	it('accumulates pages in server order and never reorders them', () => {
		let feed = appendPage(EMPTY_RECENT_FEED, page([row(9), row(8)], 'c1'));
		feed = appendPage(feed, page([row(7), row(6)], 'c2'));
		feed = appendPage(feed, page([row(5)]));
		expect(feed.items.map((i) => i.id)).toEqual([9, 8, 7, 6, 5]);
		expect(nextRequest(feed, 200)).toBeUndefined();
	});

	it('walks a whole list to exhaustion in a bounded number of presses', () => {
		// The loop a screen runs, driven only by nextRequest. If the stop rule
		// were wrong in either direction this either never terminates or
		// terminates early, and both are visible here.
		const server: { rows: RecentItem[]; cursor?: string }[] = [
			{ rows: [row(1)], cursor: 'c1' },
			{ rows: [], cursor: 'c2' },
			{ rows: [row(2), row(3)], cursor: 'c3' },
			{ rows: [row(4)] }
		];
		let feed = EMPTY_RECENT_FEED;
		let presses = 0;
		for (;;) {
			const request = nextRequest(feed, 200);
			if (request === undefined) break;
			const next = server[presses];
			presses += 1;
			expect(presses).toBeLessThanOrEqual(server.length);
			feed = appendPage(feed, page(next.rows, next.cursor));
		}
		expect(presses).toBe(4);
		expect(feed.items.map((i) => i.id)).toEqual([1, 2, 3, 4]);
	});
});

/**
 * THE POSTER KEY AND THE URL BUILT FROM IT — ARCHITECTURE.md §4.4's serving
 * half, client side.
 *
 * ⚠️ THE ABSENT CASE IS NO LONGER THE ONLY CASE. This used to say every row of
 * every real install was absent, because the fetch half of the image pipeline
 * was not built and nothing wrote `image_asset`. It is built: `internal/store`'s
 * `PutPosterAsset` writes the row, and `internal/libsync`'s phase D calls it
 * once per imported book on a BookOrbit import. Absence stays ordinary all the
 * same — any other adapter, any work imported before that pass, any cover that
 * 404'd — so both paths below are live, and the absent one is still the one a
 * renderer must handle first.
 */
describe('the poster key', () => {
	it('is absent, not empty, when the server omits it', () => {
		const item = toRecentItem(wireRow({ poster_key: undefined }));
		expect(item).toBeDefined();
		expect('posterKey' in (item as RecentItem)).toBe(false);
		expect(posterUrl(item as RecentItem)).toBeUndefined();
	});

	it('builds the /img URL the server routes', () => {
		const item = toRecentItem(wireRow()) as RecentItem;
		expect(posterUrl(item)).toBe('/img/aaaaaaaaaaaaaaaa?w=orig');
		expect(posterUrl(item, '342')).toBe('/img/aaaaaaaaaaaaaaaa?w=342');
	});

	it('offers exactly the seven widths the server accepts', () => {
		// internal/imagecache's allowlist, verbatim. A width invented here is a
		// 400 and a broken image, not a resized one.
		expect([...IMAGE_WIDTHS]).toEqual(['92', '154', '200', '342', '500', '780', 'orig']);
	});

	it('refuses to build a URL from a value that is not a cache key', () => {
		// The server validates at the read AND at the response boundary, so
		// none of these can arrive from it. They are re-checked because "the
		// server validated it" is a claim about a different process, and this
		// value becomes a URL path segment.
		for (const bad of [
			'',
			'..',
			'../../etc/passwd',
			'AAAAAAAAAAAAAAAA',
			'aaaaaaaaaaaaaaa',
			'aaaaaaaaaaaaaaaaa',
			'aaaaaaaaaaaaaaa/',
			'aaaaaaaaaaaaaa%00'
		]) {
			const item = toRecentItem(wireRow({ poster_key: bad })) as RecentItem;
			expect(posterUrl(item)).toBeUndefined();
		}
	});
});
