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
 * WHAT THE SERVER DOES NOT SERVE YET, so that nothing here codes against it:
 * `?lib=` library scoping, cover art, and Blocks A and B of §17.2. All three
 * are named as absent in `internal/httpapi/library.go`'s own header.
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
 * `haveCell` below has a rendering for it that cannot invent a denominator.
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
	/** `have == 0`. Nothing of this is held. */
	| { k: 'none' }
	/** An honest denominator and a gap. */
	| { k: 'fraction'; have: number; total: number }
	/** A count with no honest denominator. Never a tick. */
	| { k: 'partial'; have: number };

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
	 * `toAvailability`, and note that absence is ordinary rather than an error. */
	availability?: Availability;
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
 * ⚠️ A ROW WITH NO AVAILABILITY BLOB NEVER GETS A TICK, AND THE TEMPTING BUG IS
 * WORTH NAMING. `have_count` and `want_count` are always on the wire, so
 * `total = have + want` looks like a free denominator — and on every row whose
 * `want_count` is 0 it would render ✓, i.e. "you have all of it", from two
 * numbers that never said so. `want_count` is what is WANTED and absent, not
 * what exists. So a blob-less row prints its count and its gap and claims
 * nothing about completeness.
 */
export function haveCell(item: RecentItem, max = 3): HaveCellModel {
	const missing = item.wantCount > 0 ? `${item.wantCount} missing` : '';
	const availability = item.availability;

	if (availability === undefined) {
		return {
			lines: [
				{
					key: 'have',
					label: '',
					// `null`, deliberately: see the header. There is no denominator.
					mark: availabilityMark(item.haveCount, null)
				}
			],
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
 */
export function cursorRejected(error: unknown): boolean {
	return error instanceof ApiError && error.status === 400 && error.code === 'bad_request';
}
