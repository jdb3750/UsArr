/**
 * HOME'S BLOCK C, CLIENT SIDE — `GET /api/v1/library/recent`.
 *
 * ARCHITECTURE.md §17.2 as amended by ADR-0028: ONE unified recently-added
 * table across every media type, newest first, keyset-paginated. Not one strip
 * per media type and not one request per media type.
 *
 * DOM-free and deterministic, like `$lib/home` and `$lib/list`, so the
 * node-environment vitest run can pin every rule in it. `vitest.config.ts` is
 * `environment: 'node'` with no Svelte plugin, so a rule that lives inside an
 * `{#if}` in `routes/+page.svelte` is a rule nothing can test — which is why
 * the paging stop condition, the six-value type vocabulary and the whole of
 * §6.3's availability rendering are functions here rather than markup there.
 *
 * IT IS A LOCAL READ (principle 1). `internal/httpapi/library.go` says so at
 * its own declaration: one SQLite statement per page, no *Arr, no metadata
 * provider, no image fetch. Nothing in this module belongs on a path that waits
 * for an upstream, and there is no upstream behind it to wait for.
 *
 * WHAT THIS ENDPOINT DOES NOT SERVE, so that nothing here codes against it:
 * Blocks A and B of §17.2, and no `?lib=` library scope. ⚠️ THE TWO BLOCKS ARE
 * NOT ALIKE, AND THIS ONCE SAID SERVER.GO ROUTED NEITHER. Block A is routed —
 * `GET /api/v1/library/facets`, registered in `internal/httpapi/server.go`'s
 * route table and handled by `handleLibraryFacets` — and THIS MODULE EXPORTS
 * ITS URL, as `LIBRARY_FACETS_URL` below. What is true of Block A is only that
 * the recent read does not serve it; it has a read of its own. Block B is the
 * one with no route at all.
 *
 * ⚠️ THE `?lib=` SCOPE IS A PROPERTY OF THIS ENDPOINT AND NOT OF THE SERVER.
 * It is served by `GET /api/v1/library` (`http-api.md` §7.3), which this module
 * does not call; a client wanting the §17.2 scope chip goes there, not here (§17.8
 * configures a library; the chip that SCOPES to one is §17.2's axes table,
 * which files it under `scope` and specifies it as the multi-select above the
 * nav).
 * `library.go`'s header carries the same split at its own declaration.
 *
 * ⚠️ COVER ART USED TO BE ON THAT LIST AND IS NOT ANY MORE. This endpoint
 * serves `poster_key` — `image_asset.cache_key`, the key `GET /img/{key}`
 * addresses — and `posterUrl` below builds the URL. ⚠️ AND THE BYTES USED TO BE
 * ON IT TOO. This said the fetch half of the image pipeline was not built and
 * nothing wrote `image_asset`, so the key was omitted on every row of every real
 * install; that is false now. `internal/imagepipeline` fetches and renders a
 * poster, `internal/store`'s `PutPosterAsset` records the row, and
 * `internal/libsync`'s phase D (`covers.go`) calls it once per imported book on
 * a BookOrbit import. A renderer must STILL handle the absent case first,
 * because absence stays ordinary rather than exceptional: no other adapter
 * fetches a cover, nothing backfills a work imported before that pass existed,
 * and a cover the credential got a 404 for is deliberately not recorded.
 */

import { ApiError, getJson } from './api';

/** The endpoint, as `internal/httpapi/server.go` routes it. */
export const LIBRARY_RECENT_URL = '/api/v1/library/recent';

/* ── 1. the media-type vocabulary ─────────────────────────────────────────── */

/**
 * §17.2's navigation enum, closed at six, and the exact strings
 * `internal/store/recent.go` publishes as `MediaTypeMovies` … `MediaTypeComics`.
 *
 * ⚠️ THE TYPE COLUMN RENDERS FROM `media_type` AND NEVER FROM `kind`, AND THAT
 * IS A CORRECTNESS RULE RATHER THAN A PREFERENCE. §17.2 states that the Tier 1
 * client prefix index carries no format at all, so *"the Ebooks/Audiobooks
 * split is server-side only in v0.1"*: a browser holding `kind` cannot tell an
 * ebook from an audiobook, because both are `kind: 'book'` and what separates
 * them is `edition.format`, which is not on this wire. Deriving the cell from
 * `kind` would silently collapse two of the six chips into one.
 */
export const MEDIA_TYPES = ['movies', 'tv', 'music', 'ebooks', 'audiobooks', 'comics'] as const;

export type MediaType = (typeof MEDIA_TYPES)[number];

const MEDIA_TYPE_LABEL: Record<MediaType, string> = {
	movies: 'Movies',
	tv: 'TV',
	music: 'Music',
	ebooks: 'Ebooks',
	audiobooks: 'Audiobooks',
	comics: 'Comics'
};

export function isMediaType(value: string): value is MediaType {
	return (MEDIA_TYPES as readonly string[]).includes(value);
}

/**
 * The Type cell's text.
 *
 * A value outside the six is rendered VERBATIM rather than dropped or replaced
 * with a nothing-word. The server resolves this field from a closed switch and
 * errors rather than emitting a seventh (`mediaTypeOf` in
 * `internal/store/recent.go` returns `""` for an unmapped kind and the read
 * fails), so a seventh reaching a browser means the enum grew — and a row that
 * says `games` is the honest rendering of that, where an em dash would hide it.
 */
export function mediaTypeLabel(value: string): string {
	return isMediaType(value) ? MEDIA_TYPE_LABEL[value] : value;
}

/* ── 2. the availability blob ─────────────────────────────────────────────── */

/**
 * ONE BUCKET OF A TIER- OR EDITION-KEYED ROLLUP.
 *
 * `total` is `number | null` and the null is load-bearing: `docs/reference/
 * schema.md` is explicit that **`total: null` is not `total: 0`** — the first
 * means "nobody honestly knows", the second means "the series is empty" — and
 * that §6.3's tick *"must never fire on the first"*.
 */
export interface AvailabilityBucket {
	/** The blob's own key: a tier name, or an `edition:…` identifier. */
	key: string;
	/** What the renderer puts beside the fraction, or '' when there is nothing
	 * honest to put there. A tier keys ITSELF (`1080p`); an edition key is an
	 * opaque `edition:mbz_release:abc-123`, so its label is the blob's `label`
	 * and an edition with none gets a bare fraction rather than an id. */
	label: string;
	have: number;
	total: number | null;
}

/**
 * THE POLYMORPHIC ROLLUP, AS A DISCRIMINATED UNION ON `k`.
 *
 * `docs/reference/schema.md` ("The availability blob, per medium") carries the
 * three shapes and the reason there is a discriminator at all: *"Without one, a
 * renderer cannot tell a tier key from an edition key in the same object."*
 *
 *   tier     video. `total` is a property of the parent work (§6.3).
 *   edition  music. `total` is a property of the EDITION, because choosing the
 *            2017 remaster over the 2000 original changes the track list
 *            (ADR-0031), so a bare fraction is a guess.
 *   count    comics, and anything else with no honest denominator. `total` is
 *            present only where the series is ended AND a total was declared.
 *
 * `internal/httpapi/library.go` forwards the column VERBATIM as
 * `json.RawMessage` and parses nothing, so this module is the first thing on
 * either side of the wire that reads the shape.
 */
export type Availability =
	| { k: 'tier'; buckets: AvailabilityBucket[] }
	| { k: 'edition'; buckets: AvailabilityBucket[] }
	| {
			k: 'count';
			have: number;
			total: number | null;
			/** Where a declared total came from, because every total in this domain
			 * is a declaration rather than a fact. Absent when `total` is. */
			totalSource: string;
			/** Contiguity gaps, computed locally: `["7","12","30-32"]`. */
			missing: string[];
	  };

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function num(value: unknown): number | undefined {
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function str(value: unknown): string {
	return typeof value === 'string' ? value : '';
}

/** `have` and `total` out of one `{have, total}` object, with `total` kept
 * nullable. A non-number `total` — absent, `null`, or anything else — becomes
 * `null` and can therefore never satisfy the tick's `total > 0`. */
function bucketOf(key: string, label: string, value: unknown): AvailabilityBucket | undefined {
	if (!isRecord(value)) return undefined;
	const have = num(value.have);
	if (have === undefined) return undefined;
	return { key, label, have, total: num(value.total) ?? null };
}

/**
 * The blob, parsed, or `undefined` when there is nothing to switch on.
 *
 * ⚠️ `undefined` IS A REAL AND EXPECTED ANSWER, not a parse failure to shrug
 * at. `internal/httpapi/library.go` omits `availability` when the column is
 * NULL **and** when the stored text is not valid JSON — it drops the second
 * case rather than forwarding it, because one malformed historical blob would
 * otherwise fail the whole response. So a row with no rollup is ordinary, and
 * `haveCell` below renders it as `uncounted`: no denominator, and no count
 * either, because http-api.md §1.4.1 makes absence a statement about the
 * ROLLUP rather than about the library.
 *
 * An UNRECOGNISED `k` also lands here. schema.md makes `k` required on every
 * non-null blob precisely so a v0.1 writer stays forward-compatible; a fourth
 * medium's shape is not something this renderer can guess at, and guessing
 * would put a wrong fraction in the one column the user scans for what is
 * missing.
 */
export function toAvailability(value: unknown): Availability | undefined {
	if (!isRecord(value)) return undefined;
	const k = value.k;
	if (k === 'count') {
		const have = num(value.have);
		if (have === undefined) return undefined;
		const missing = Array.isArray(value.missing)
			? value.missing.filter((m): m is string => typeof m === 'string' && m.length > 0)
			: [];
		return {
			k: 'count',
			have,
			// ⚠️ `?? null`, NEVER `?? 0`. See AvailabilityBucket.total.
			total: num(value.total) ?? null,
			totalSource: str(value.total_source),
			missing
		};
	}
	if (k !== 'tier' && k !== 'edition') return undefined;

	const buckets: AvailabilityBucket[] = [];
	for (const [key, entry] of Object.entries(value)) {
		if (key === 'k') continue;
		// A tier keys itself and is its own label; an edition's key is opaque, so
		// only the blob's `label` can name it.
		const label = k === 'tier' ? key : str(isRecord(entry) ? entry.label : '');
		const bucket = bucketOf(key, label, entry);
		if (bucket !== undefined) buckets.push(bucket);
	}
	if (buckets.length === 0) return undefined;
	return { k, buckets };
}

/* ── 3. §6.3's render rule ────────────────────────────────────────────────── */

/**
 * WHAT ONE FRACTION RENDERS AS. ARCHITECTURE.md §6.3, verbatim: *"Render
 * `have == total && total > 0` → ✓; `have == 0` → ✗; otherwise the fraction."*
 *
 * ⚠️ EVERY ARM HERE IS ABOUT A PRESENT BLOB. This function is never the answer
 * for a work whose `availability` key was absent — that is `uncounted`, and
 * `haveCell` decides it before reaching here. Calling this with an absent blob's
 * `have_count` is the bug http-api.md §1.4.1 exists to prevent: it returns
 * `none` on the `0` every uncounted row carries by column default.
 *
 * The fourth arm is this module's, and it exists because §6.3's three do not
 * cover the shape schema.md added under `k: "count"`: `have` with a `total` of
 * `null`. It is NOT a fraction — there is no denominator — and it is NOT
 * complete, which is the whole point of schema.md's rule that the tick *"must
 * never fire"* on a null total. So it is its own state and it prints the bare
 * count.
 */
export type AvailabilityMark =
	/** `have == total && total > 0`. The muted ✓ in neutral text (§17.2). */
	| { k: 'complete' }
	/**
	 * `have == 0` ON A PRESENT BLOB, and on nothing else. This is the truthful
	 * zero: something counted this work and the answer was none of it.
	 * http-api.md §1.4.1 puts §6.3's `have == 0` → ✗ on a present blob
	 * *"and on nothing else"*, which is what keeps it apart from `uncounted`.
	 */
	| { k: 'none' }
	/** An honest denominator and a gap. */
	| { k: 'fraction'; have: number; total: number }
	/** A count with no honest denominator. Never a tick. */
	| { k: 'partial'; have: number }
	/**
	 * ⚠️ NO COUNT HAS EVER BEEN COMPUTED FOR THIS WORK. The `availability` key
	 * was absent from the wire, which `docs/reference/http-api.md` §1.4.1 defines
	 * as *"no count has ever been computed for that work"* — not a zero, and not a
	 * statement about the library.
	 *
	 * ⚠️ `availabilityMark` CANNOT RETURN IT, DELIBERATELY. The fact that decides
	 * this state is the PRESENCE of the blob, not a number, so it is decided at
	 * `haveCell` where presence is known and no sentinel has to travel through
	 * `have` or `total`. `have_count` is sent unconditionally and its column is
	 * `NOT NULL DEFAULT 0`, so a `0` in it is not evidence of anything on its own.
	 */
	| { k: 'uncounted' };

export function availabilityMark(have: number, total: number | null): AvailabilityMark {
	// The tick is tested FIRST and its `total > 0` clause is tested against a
	// number, so a null total cannot reach it however `have` compares. `null`
	// coerces to 0 in a loose comparison, which is exactly the bug schema.md
	// names — hence a typed guard rather than `have === total`.
	if (total !== null && total > 0 && have === total) return { k: 'complete' };
	if (have === 0) return { k: 'none' };
	if (total !== null && total > 0) return { k: 'fraction', have, total };
	return { k: 'partial', have };
}

/* ── 4. one row off the wire ──────────────────────────────────────────────── */

/**
 * One Block C row, as `recentWorkResponse` in `internal/httpapi/library.go`
 * spells it. That struct is a hand-written allowlist, so this is the whole
 * wire and not a projection of a larger record.
 */
export interface RecentItem {
	/** `work.id`. Published deliberately — §4.5's Tier 1 index already ships it
	 * and item routes are `/library/{type}/{id}` — and it is NOT shaped like
	 * `provenance.id`, which the grabs endpoint publishes as an HMAC. */
	id: number;
	/** One of MEDIA_TYPES, resolved server-side. See MEDIA_TYPES. */
	mediaType: string;
	/** `work.kind` verbatim. It travels BESIDE mediaType rather than instead of
	 * it: kind is the schema's word and is what an item route needs, media type
	 * is the user's word and is what the Type cell renders. */
	kind: string;
	title: string;
	/**
	 * ⚠️ GENUINELY OPTIONAL. The server omits the key rather than sending `0`,
	 * because *"a rendered 0 is a claim about a release date; an absent key is
	 * the truth"*. Rendering it as `0`, or as `1970`, re-invents exactly what
	 * the server went to the trouble of not saying.
	 */
	year?: number;
	/**
	 * ⚠️ ALSO GENUINELY OPTIONAL, and its absence is a state Kavita reaches:
	 * `internal/libsync/kavita.go` writes this from the upstream's `created`
	 * field and stores a zero time as NULL. An undated row sorts LAST, never
	 * first — the block answers "what did I just get", and a row with no date
	 * must not be able to claim the top of it.
	 *
	 * RFC 3339, or absent where the stored value would not parse: the server
	 * drops an unparseable timestamp rather than guessing, because the whole
	 * column is an ordering claim.
	 */
	addedAt?: string;
	/** `work`'s denormalised rollups: the numerator, and the gap. */
	haveCount: number;
	wantCount: number;
	/** Absent where the column is NULL or its text would not parse. See
	 * `toAvailability`, and note that absence is ordinary rather than an error:
	 * http-api.md §1.4.1 makes it *"not counted"*, never *"none held"*. */
	availability?: Availability;
	/**
	 * `image_asset.cache_key` for the work's poster: the key `GET /img/{key}`
	 * addresses. `posterUrl` turns it into a URL; nothing else may.
	 *
	 * ⚠️ GENUINELY OPTIONAL, AND THE REASON HAS CHANGED. This used to say it was
	 * absent on EVERY row of every real install because nothing wrote
	 * `image_asset`; `internal/store`'s `PutPosterAsset` writes it, called once
	 * per imported book by `internal/libsync`'s phase D on a BookOrbit import, so
	 * the key is populated for works that pass got a cover for. Absence is still
	 * ORDINARY — any other adapter, any work imported before that pass, any cover
	 * that 404'd — rather than evidence of a broken route. A renderer treats
	 * absence as "this work has no artwork" and draws whatever it draws for that
	 * — it must never substitute an empty string and build `/img/` from it.
	 */
	posterKey?: string;
}

/**
 * The `?w=` allowlist from ARCHITECTURE.md §4.4, as `internal/imagecache`
 * publishes it.
 *
 * ⚠️ IT IS AN ALLOWLIST BECAUSE THE SERVER REFUSES ANYTHING ELSE — an arbitrary
 * `?w=` is a cache-poisoning DoS (GHSA-rrr6-mvwg-9pg9) — so a width invented
 * here produces a 400 and a broken image, not a resized one.
 */
export const IMAGE_WIDTHS = ['92', '154', '200', '342', '500', '780', 'orig'] as const;

export type ImageWidth = (typeof IMAGE_WIDTHS)[number];

/**
 * The URL for one poster at one width, or `undefined` when the item has no
 * artwork.
 *
 * RETURNING `undefined` RATHER THAN A PLACEHOLDER URL IS THE POINT. A caller
 * that gets a string always would render an `<img>` that 404s on every row of
 * every install today, which is worse than the text-only row it replaced. The
 * absent case is the caller's to draw.
 *
 * The key is NOT escaped and does not need to be: `internal/store` refuses
 * anything that is not sixteen lowercase hex characters, at the read AND at the
 * response boundary, so a value that reached here is already URL-safe. It is
 * re-checked anyway, because "the server validated it" is a claim about a
 * different process.
 */
export function posterUrl(item: RecentItem, width: ImageWidth = 'orig'): string | undefined {
	const key = item.posterKey;
	if (key === undefined || !/^[0-9a-f]{16}$/.test(key)) return undefined;
	return `/img/${key}?w=${width}`;
}

/**
 * THE WIDTH THE POSTER GRID ASKS FOR, out of `IMAGE_WIDTHS` and not out of
 * arithmetic. A grid column is about 150 CSS px, which is 300 device px on the
 * 2× displays this is mostly read on; `342` is the first allowlisted width that
 * covers that without asking the server for a size it refuses.
 */
export const POSTER_GRID_WIDTH: ImageWidth = '342';

/**
 * ONE POSTER CARD, DECIDED HERE RATHER THAN IN THE TEMPLATE.
 *
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so a rule
 * written as an expression inside an `{#each}` is a rule nothing can test —
 * which is why `$lib/home`, `$lib/search` and this module already hold their
 * screens' decisions. The decisions here are which URL the tile draws and what
 * the native tooltip says, and both can be got wrong silently.
 */
export interface PosterTile {
	/** `work.id`, the each-block key. */
	id: number;
	/** The title verbatim. Empty is a real state and the caller draws it as one,
	 * exactly as the table's Title cell does. */
	title: string;
	year?: number;
	/**
	 * `posterUrl` at `POSTER_GRID_WIDTH`, ABSENT where the work has no artwork.
	 * `posterUrl` returns `undefined` on purpose so the absent case is the
	 * caller's to draw, and it stays absent here rather than becoming an empty
	 * string: `<img src="">` re-requests the current document.
	 */
	src?: string;
	/**
	 * The full title, for the native `title` attribute DESIGN-DIRECTION §9.2
	 * puts on both the art and the title line — the card's title is one
	 * ellipsised line, so the untruncated string has to be reachable somewhere.
	 * Absent where there is no title, because `title=""` is a tooltip promising
	 * nothing.
	 */
	tooltip?: string;
}

/** One card's worth of a recent item. */
export function posterTile(item: RecentItem): PosterTile {
	const tile: PosterTile = { id: item.id, title: item.title };
	if (item.year !== undefined) tile.year = item.year;
	const src = posterUrl(item, POSTER_GRID_WIDTH);
	if (src !== undefined) tile.src = src;
	if (item.title !== '') tile.tooltip = item.title;
	return tile;
}

/**
 * THE URL A CARD SHOULD ACTUALLY PUT IN ITS `<img>`, OR `undefined` FOR THE
 * EMPTY TILE.
 *
 * ⚠️ A KEY IS NOT A PROMISE THAT BYTES EXIST, AND THAT IS THE DEFECT THIS
 * ANSWERS. `GET /img/{key}` is a cache read and never fetches upstream, so a
 * work whose poster has not been rendered yet answers `404 not_cached` — an
 * ordinary state, because the key is written by the catalogue import and the
 * bytes by a separate pass. An `<img>` with no error handling then draws the
 * browser's own broken-image glyph. On Home that is a handful of tiles; on a
 * screenful of covers it is the whole screen reading as broken.
 *
 * SO A FAILED LOAD COLLAPSES INTO THE ABSENT-KEY CASE, which already has a
 * rendering: the bordered tile filled from `--dc`. The two are different facts
 * about the pipeline and the same fact on screen — there is no art — and giving
 * them two renderings would put two spellings of "no cover" on one row.
 *
 * THE RECORD IS THE CALLER'S, held for the page view. This function is the rule
 * and holds nothing, so `vitest.config.ts`'s node environment can call it: the
 * failure is discovered in an `onerror` a component owns, and a rule left in
 * that handler is a rule nothing can test.
 *
 * ⚠️ A KEYED RECORD AND NOT A `SvelteSet`, AND THE REASON IS THE RENDER PATH
 * RATHER THAN TASTE. In `svelte@5.56.9`'s `src/reactivity/set.js`, `has()` on a
 * MISS subscribes the reader to the SET-WIDE `#version` signal — the source says
 * so in as many words: *"If the value doesn't exist, track the version in case
 * it's added later but don't create sources willy-nilly to track all possible
 * values"* — and `add()` calls `increment(this.#version)`. Every card whose
 * cover is fine is a miss, so one broken cover invalidates all of them and
 * re-runs the tile build for the whole grid: O(K·N) over K failures and N cards,
 * on a page "Load more" grows without bound. A `$state`-proxied record has
 * per-key granularity instead — `src/internal/client/proxy.js`'s `get` trap
 * creates a source for an ABSENT property too (`if (s === undefined && (!exists
 * || …))`, seeded `UNINITIALIZED` and returned as `undefined`), and the
 * `version` a new key increments is read only by `ownKeys`, which a card that
 * reads one id never touches.
 */
export function posterArtSrc(
	tile: PosterTile,
	failed: Readonly<Record<number, true | undefined>>
): string | undefined {
	return tile.src !== undefined && failed[tile.id] === undefined ? tile.src : undefined;
}

/**
 * One row, or `undefined` for a frame this screen cannot draw.
 *
 * The id is the only required field: it is the row key and the item route, and
 * a row with neither can be neither identified nor followed. A missing title is
 * NOT a reason to drop the row — the cell has a rendering for an empty one, and
 * dropping it would make the table silently short by rows the server sent.
 */
export function toRecentItem(value: unknown): RecentItem | undefined {
	if (!isRecord(value)) return undefined;
	const id = num(value.id);
	if (id === undefined) return undefined;
	const item: RecentItem = {
		id,
		mediaType: str(value.media_type),
		kind: str(value.kind),
		title: str(value.title),
		// `?? 0` is right for these two and wrong for `year`: the server sends
		// both unconditionally (no `omitempty`), so an absent one is a broken
		// frame rather than a fact, and 0 held is a true statement about it.
		haveCount: num(value.have_count) ?? 0,
		wantCount: num(value.want_count) ?? 0
	};
	// Assigned only when present, so `exactOptionalPropertyTypes`-style absence
	// survives into the render rather than becoming `undefined`-valued keys.
	const year = num(value.year);
	if (year !== undefined) item.year = year;
	const addedAt = str(value.added_at);
	if (addedAt !== '') item.addedAt = addedAt;
	const availability = toAvailability(value.availability);
	if (availability !== undefined) item.availability = availability;
	// Assigned only when the server sent one. `str()` yields '' for an absent
	// key, and '' is not a poster key — it is the absence, which must stay
	// distinguishable so `posterUrl` can return undefined rather than `/img/`.
	const posterKey = str(value.poster_key);
	if (posterKey !== '') item.posterKey = posterKey;
	return item;
}

/* ── 5. the Have cell ─────────────────────────────────────────────────────── */

/** One rendered line of the Have cell: an optional label and its mark. */
export interface HaveLine {
	key: string;
	label: string;
	mark: AvailabilityMark;
}

export interface HaveCellModel {
	lines: HaveLine[];
	/** Buckets beyond the cap, for §9.1's `+N more`. */
	more: number;
	/** §17.2's `· N missing`, in the warn role. '' when nothing is wanted. */
	missing: string;
	/** `k: "count"`'s own contiguity list, already joined. '' when there is
	 * none. This is the number that is always honest — it is computed locally
	 * from `work_comic_issue.number_sort` with no upstream help. */
	gaps: string;
}

/**
 * THE HAVE COLUMN, DERIVED. §17.2's grammar is `have / total · N missing`, with
 * a complete row rendered as a muted ✓ and an incomplete row carrying its gap
 * figure in the warn role.
 *
 * ⚠️ A ROW WITH NO AVAILABILITY BLOB IS `uncounted`, AND IT CLAIMS NOTHING —
 * NEITHER A TICK NOR A CROSS. `docs/reference/http-api.md` §1.4.1: the absence
 * of `availability` *"means no count has ever been computed for that work"*, and
 * a consumer *"must not render an absent blob as `0`, as \"none\", or as any
 * glyph, bar or accessible name that asserts emptiness"*. Both temptations are
 * worth naming, because this cell fell for the second one:
 *
 *   the invented denominator  `have_count` and `want_count` are always on the
 *     wire, so `total = have + want` looks free — and on every row whose
 *     `want_count` is 0 it renders ✓, "you have all of it", out of two numbers
 *     that never said so. `want_count` is what is WANTED and absent, not what
 *     exists.
 *   the invented zero  passing `have_count` to `availabilityMark` looked like
 *     "show what it knows", and `have_count` is `NOT NULL DEFAULT 0`, so every
 *     work nobody has counted arrived at `have === 0` → ✗ *none held* — a claim
 *     about a library the reader has never measured.
 *
 * ⚠️ AND IT SELF-CORRECTS WITH NO SECOND EDIT WHEN THE ROLLUP SHIPS. A truthful
 * zero arrives as a PRESENT blob carrying `have: 0`, which is `k: 'count'` below
 * and reaches `availabilityMark` as it always did, so the same code then renders
 * a real ✗. Absence keeps meaning "not counted" after the rollup lands, because
 * the counts and the blob are one recompute over one dirty bit (`work.
 * rollup_dirty`, ARCHITECTURE §6.3): there is no specified writer that moves
 * `have_count` while leaving the key absent.
 */
export function haveCell(item: RecentItem, max = 3): HaveCellModel {
	const missing = item.wantCount > 0 ? `${item.wantCount} missing` : '';
	const availability = item.availability;

	if (availability === undefined) {
		return {
			// ⚠️ NOT `availabilityMark(item.haveCount, …)`. Presence of the blob is
			// the whole of the question here, so no count is consulted at all: see
			// the header, and `AvailabilityMark`'s `uncounted` arm.
			lines: [{ key: 'have', label: '', mark: { k: 'uncounted' } }],
			more: 0,
			missing,
			gaps: ''
		};
	}

	if (availability.k === 'count') {
		return {
			lines: [
				{ key: 'count', label: '', mark: availabilityMark(availability.have, availability.total) }
			],
			more: 0,
			missing,
			gaps: availability.missing.join(', ')
		};
	}

	// §9.1 caps a cell that renders one line per related object at three plus
	// `+N more`; a dual-Radarr work has two tiers and a well-catalogued album can
	// have many editions.
	const shown = availability.buckets.slice(0, max);
	return {
		lines: shown.map((b) => ({
			key: b.key,
			label: b.label,
			mark: availabilityMark(b.have, b.total)
		})),
		more: availability.buckets.length - shown.length,
		missing,
		gaps: ''
	};
}

/* ── 6. the keyset page, and the one place the stop rule lives ────────────── */

/** What one page request carries. `cursor` absent is "the newest items". */
export interface RecentRequest {
	limit: number;
	cursor?: string;
}

export interface RecentPage {
	items: RecentItem[];
	/**
	 * The limit the SERVER applied, echoed because it clamps: `handleRecentWorks`
	 * defaults a non-positive limit to `RecentWorksDefaultLimit` (50) and caps
	 * anything above `RecentWorksMaxLimit` (200). A client that asked for 10,000
	 * and got 200 rows must not read the short answer as "that is all there is".
	 */
	limit: number;
	/**
	 * ⚠️ ABSENT MEANS THERE IS NO NEXT PAGE, AND IT IS THE ONLY THING THAT DOES.
	 * `next_cursor` is `omitempty` on the server and the server mints it from a
	 * probe row rather than from a guess, so its absence is an observation.
	 */
	nextCursor?: string;
}

export function toRecentPage(payload: unknown, limit: number): RecentPage {
	if (!isRecord(payload)) return { items: [], limit };
	const raw = Array.isArray(payload.items) ? payload.items : [];
	const page: RecentPage = {
		items: raw.map(toRecentItem).filter((i): i is RecentItem => i !== undefined),
		limit: num(payload.limit) ?? limit
	};
	const next = str(payload.next_cursor);
	if (next !== '') page.nextCursor = next;
	return page;
}

/** The URL for one page. The cursor is percent-encoded rather than
 * concatenated: it is base64url of a payload that embeds a SQLite datetime with
 * a literal space in it, and the first test that walked two pages of this
 * endpoint caught the unescaped form as a panic rather than as a wrong answer
 * (`EncodeRecentWorksCursor` in `internal/store/recent.go`). */
export function recentRequestUrl(request: RecentRequest): string {
	const params = new URLSearchParams({ limit: String(request.limit) });
	if (request.cursor !== undefined) params.set('cursor', request.cursor);
	return `${LIBRARY_RECENT_URL}?${params.toString()}`;
}

/** One page of Block C. A local SQLite read behind the endpoint: it makes no
 * upstream call and is never on a path that waits for a service. */
export async function fetchRecentPage(request: RecentRequest): Promise<RecentPage> {
	return toRecentPage(await getJson(recentRequestUrl(request)), request.limit);
}

/**
 * EVERY PAGE READ SO FAR, PLUS THE ONE FACT THAT DECIDES WHETHER THERE IS
 * ANOTHER.
 *
 * `cursor` is the whole of the paging state and `loaded` is the whole of the
 * "has anything been read yet" state. There is deliberately no `hasMore`
 * boolean beside them: two fields that must agree are two fields that can
 * disagree, and the rule this type exists to hold is that "is there more" is
 * `cursor !== undefined` and nothing else.
 */
export interface RecentFeed {
	items: RecentItem[];
	/** The cursor for the NEXT page. Absent = this is the last page. */
	cursor?: string;
	/** Whether the server has answered at least once. An empty list and an
	 * unread list mean opposite things, exactly as they do for recent grabs. */
	loaded: boolean;
}

export const EMPTY_RECENT_FEED: RecentFeed = { items: [], loaded: false };

/**
 * ⚠️ THE STOP RULE, IN ONE PLACE, AND THE BUG IT CLOSES IS THE REASON IT IS A
 * FUNCTION RATHER THAN AN `{#if}` ON THE SCREEN.
 *
 * **Stop when `next_cursor` is ABSENT. Never infer "done" from a short page.**
 * `ListRecentWorks` walks the dated range and, at the one boundary where that
 * range runs out, issues a second statement for the undated tail — so a page
 * can legitimately come back shorter than the limit with more rows still to
 * come. A client that stopped on `items.length < limit` would truncate the
 * table at exactly the row whose upstream reported no creation date, silently
 * and forever.
 *
 * Returns the request for the next page, or `undefined` when there is none.
 * A caller that gets `undefined` has nothing left to ask for, and that is the
 * same answer whether the list is exhausted or has never been read at all —
 * which is why `loaded` is checked first.
 */
export function nextRequest(feed: RecentFeed, limit: number): RecentRequest | undefined {
	if (!feed.loaded) return { limit };
	if (feed.cursor === undefined) return undefined;
	return { limit, cursor: feed.cursor };
}

/** Whether "Load more" has anything to press for. Derived, never stored. */
export function hasMore(feed: RecentFeed): boolean {
	return feed.cursor !== undefined;
}

/** The feed with one page appended. Immutable, so a Svelte `$state` assignment
 * is what re-renders rather than a mutation nothing observes. */
export function appendPage(feed: RecentFeed, page: RecentPage): RecentFeed {
	const next: RecentFeed = { items: [...feed.items, ...page.items], loaded: true };
	if (page.nextCursor !== undefined) next.cursor = page.nextCursor;
	return next;
}

/**
 * WHETHER A FAILURE IS THE SERVER REJECTING OUR CURSOR.
 *
 * `handleRecentWorks` answers a cursor it did not issue with 400 `bad_request`
 * and an `action`, and it does that rather than silently resetting to page one
 * — *"resetting turns a stale bookmark into a Load-more loop that re-serves the
 * first page for ever and looks like the list is stuck"*. So the client must
 * not retry either: the same cursor produces the same 400. The screen surfaces
 * the server's `action` and offers to start again from the newest items, which
 * is a request with NO cursor and therefore cannot fail the same way.
 *
 * ⚠️ IT TAKES THE CURSOR THAT WAS ACTUALLY SENT, AND THAT SECOND ARGUMENT IS
 * THE WHOLE OF THE RULE RATHER THAN A CONVENIENCE. The wire cannot tell a
 * rejected cursor apart from any other bad request: `400 bad_request` is one
 * code covering several causes, and on `GET /api/v1/library` it also covers a
 * bad `media_type`, a bad `sort`, an unknown `lib` slug and a malformed
 * `limit` (http-api.md §7.7). Deciding from the status and the code alone
 * therefore tells a user whose FILTER is wrong that their bookmark has gone
 * stale, and offers them a restart that will fail in exactly the same way.
 * A request that carried no cursor cannot have had one rejected, so that case
 * is answered here rather than guessed at from the response.
 */
export function cursorRejected(error: unknown, sentCursor: string | undefined): boolean {
	if (sentCursor === undefined) return false;
	return error instanceof ApiError && error.status === 400 && error.code === 'bad_request';
}

/* ── 7. the per-media-type facet counts ───────────────────────────────────── */

/** The endpoint, as `internal/httpapi/server.go` routes it. */
export const LIBRARY_FACETS_URL = '/api/v1/library/facets';

/**
 * `GET /api/v1/library/facets`, whose whole body is six numbers.
 *
 * ⚠️ ALL SIX KEYS ARE ALWAYS PRESENT ON THE WIRE AND NONE CARRIES
 * `omitempty` — `internal/httpapi/facets.go` states that as the contract rather
 * than as a formatting accident, because *"an absent key and a zero are
 * different values a consumer would have to tell apart"*. So this interface has
 * no optional member and no `null`: there is one spelling of "none", and it is
 * `0`.
 *
 * ⚠️ AND ZERO NEVER WIDENS INTO "HIDDEN" OR "UNKNOWN". `internal/store/
 * facets.go` is categorical that a type the caller cannot see and a type with
 * genuinely no rows are INDISTINGUISHABLE here, deliberately and in one
 * direction only: restriction renders as zero. A renderer that turns a zero
 * into "hidden" would publish the existence oracle that collapse exists to
 * remove, so nothing downstream of this may do it.
 */
export interface MediaTypeCounts {
	movies: number;
	tv: number;
	music: number;
	ebooks: number;
	audiobooks: number;
	comics: number;
}

/**
 * The zero value, and the value an install with nothing catalogued really gets.
 *
 * It is six zeros rather than six `undefined`s because six zeros is already a
 * value this wire produces for a real caller — it is what a restricted scope
 * looks like — so a renderer that handles the response at all handles this. An
 * "unknown" state would be a seventh thing to render that the endpoint itself
 * refuses to have.
 *
 * ⚠️ IT IS NO LONGER WHAT A MALFORMED BODY PARSES TO, and that sentence stood
 * here until it was measured: `toMediaTypeCounts` answers `null` for a body it
 * cannot read, because six zeros is an assertion about the user's library and a
 * body the client could not read supports no assertion at all. This constant is
 * still the zero value and callers still render it as six real zeros.
 */
export const NO_MEDIA_TYPE_COUNTS: MediaTypeCounts = {
	movies: 0,
	tv: 0,
	music: 0,
	ebooks: 0,
	audiobooks: 0,
	comics: 0
};

/**
 * What a 200 whose body is not the counts envelope is called, so the screen has
 * something true to put in its own banner. There is no upstream text to quote:
 * this is UsArr talking to UsArr and getting an answer it does not recognise.
 */
export const FACETS_BODY_UNREADABLE =
	'GET /api/v1/library/facets answered, and the body was not the counts envelope it sends.';

/**
 * The envelope, narrowed. `counts` is NESTED under one key on the wire and is
 * unwrapped here; `internal/httpapi/facets.go` explains the nesting — the
 * availability rollup and the last-import time are *"their own aggregates and
 * their own commit"*, and the object leaves them somewhere to land.
 *
 * ⚠️ `null` FOR A BODY THAT IS NOT THAT SHAPE, AND THAT IS A DELIBERATE
 * DEPARTURE FROM THE HOUSE DEFAULT one line of thought above it. `toSessionState`
 * answers `SIGNED_OUT` for a body it cannot read and `toRecentPage` answers an
 * empty page, and both are right, because both of those are INERT states: a
 * signed-out screen asks the user to sign in and an empty page asks for nothing.
 * Six zeros is not inert. It is an ASSERTION ABOUT THE USER'S LIBRARY — "you
 * have no books, no audiobooks and no comics" — rendered on the one screen whose
 * whole job is to answer "what do I have?", and made off a body the client could
 * not read. A renamed envelope key would have shipped that sentence with no
 * error anywhere and the caller's own error arm unreachable.
 *
 * ⚠️ THE INSIDE OF A PRESENT `counts` STILL DEFAULTS TO ZERO, which is not the
 * same decision and is unchanged. `internal/httpapi/facets.go` sends all six
 * with no `omitempty`, so a missing or non-numeric member is a build skew rather
 * than a shape the client cannot read at all, and `0` is the value the closed
 * enum exists to make unambiguous. The line this function draws is between "the
 * server did not answer the question" and "the server answered it with a
 * number".
 */
export function toMediaTypeCounts(payload: unknown): MediaTypeCounts | null {
	if (!isRecord(payload)) return null;
	const counts = payload.counts;
	if (!isRecord(counts)) return null;
	return {
		movies: num(counts.movies) ?? 0,
		tv: num(counts.tv) ?? 0,
		music: num(counts.music) ?? 0,
		ebooks: num(counts.ebooks) ?? 0,
		audiobooks: num(counts.audiobooks) ?? 0,
		comics: num(counts.comics) ?? 0
	};
}

/**
 * The six counts. A LOCAL SQLITE READ (principle 1): `internal/store/facets.go`
 * is two statements against the local file, with no *Arr call, no metadata
 * provider and no image fetch behind it, so this never puts a render path
 * behind a service.
 *
 * IT TAKES NO ARGUMENTS BECAUSE THE ENDPOINT READS NO QUERY PARAMETER. No
 * `?lib=`, no `?media_type=`, no paging — `internal/httpapi/facets.go` lists
 * each refusal and its reason. The access scope comes off the session and a
 * caller cannot widen it, which is why it is not a parameter here either.
 *
 * `null` IS AN ANSWER AND IT IS NOT SIX ZEROS. See `toMediaTypeCounts`: a 200
 * whose body is not the envelope is a failure the caller must render as a
 * failure, and `FACETS_BODY_UNREADABLE` is the sentence for it.
 */
export async function fetchLibraryFacets(): Promise<MediaTypeCounts | null> {
	return toMediaTypeCounts(await getJson(LIBRARY_FACETS_URL));
}
