/**
 * THE LIBRARIES SCREEN, CLIENT SIDE — `GET /api/v1/libraries`.
 *
 * ARCHITECTURE.md §17.8's row view, and the wire contract is
 * `docs/reference/http-api.md` §2 rather than this file: a doc comment is not
 * reachable from a browser tab, which is the lesson §1 of that document
 * records. What is here is the parsing and the rendering decisions, in one
 * DOM-free module so the node-environment vitest run can pin them —
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so a rule
 * that lives inside an `{#if}` in `routes/libraries/+page.svelte` is a rule
 * nothing can test.
 *
 * IT IS A LOCAL READ (principle 1). `internal/httpapi/libraries.go` says so at
 * its own declaration: two SQLite statements, no *Arr, no metadata provider, no
 * capability probe. DESIGN-DIRECTION §7.2 makes that Tier 0, whose instruction
 * is "show nothing at all — no skeleton, no spinner, no fade-in", so this module
 * publishes no loading model for the screen to render.
 *
 * ⚠️ THREE FIELDS ON THIS WIRE DESCRIBE STATES NOTHING IN THE TREE CAN REACH,
 * AND THE RENDERING RULES BELOW ARE BUILT AROUND THAT RATHER THAN AROUND THE
 * FIELDS' NAMES. `http-api.md` §2.4 measures all three:
 *
 *   · `sources[].missing_since` — the two statements in non-test Go that touch
 *     the column both CLEAR it, so no code path sets a non-NULL value. ABSENCE
 *     IS THEREFORE "UNKNOWN", NEVER "HEALTHY", and `libraryStates` below emits
 *     nothing positive from it. A screen that rendered "all sources present"
 *     would be reporting the writer's silence as an observation.
 *   · `items[].orphaned_at` — no writer and no reader in non-test Go. A library
 *     with no sources is still visible as `sources: []`, which is an observation
 *     rather than an inference, so the state renders from the array and the
 *     timestamp is used only to QUALIFY it when it is present.
 *   · `items[].formats` — no writer: the column is NULL on every row, so the
 *     Ebooks/Audiobooks split it exists for is not reachable in v0.1. It is
 *     parsed here so the screen stays truthful the day something writes it, and
 *     it is not rendered as a column, because a column identical on every row is
 *     not data (DESIGN-DIRECTION §9.1).
 *
 * WHAT IS NOT ON THIS WIRE AT ALL, so that nothing here codes against it: the
 * four `sink_*` request-destination columns (§17.8 defers the whole column to
 * the first service that can be a destination, and the screen says so once in
 * its own copy), `managed_by`, `icon`, `default_sort` and the corrections. The
 * detail view is a different screen and a different read.
 *
 * AND THERE IS NO WRITE ENDPOINT BEHIND THIS SCREEN. `internal/httpapi/server.go`
 * registers one libraries route and it is a GET, so there is no Accept, no
 * Decline, no reorder, no rename and no `Add library` to build — ADR-0048
 * settles that a proposal is not a row and lives in the connect probe's
 * response, which is a different endpoint that does not exist yet either.
 */

import { ApiError, getJson } from './api';

/** The endpoint, as `internal/httpapi/server.go` routes it. */
export const LIBRARIES_URL = '/api/v1/libraries';

/* ── 1. the wire ──────────────────────────────────────────────────────────── */

/**
 * One source chip, as `librarySourceResponse` in `internal/httpapi/libraries.go`
 * spells it. That struct is a hand-written field-by-field allowlist, so this is
 * the whole wire and not a projection of a larger record.
 *
 * ⚠️ NOTHING SERVICE-SIDE BEYOND A NAME AND A KIND CROSSES, AND NOTHING CAN.
 * The row joins to `service_instance`, which carries `api_key_enc` and
 * `base_url`; exactly two of that table's columns are read and exactly two reach
 * the browser, and `TestListLibrariesShipsNoCredentialOrAddress` holds it there.
 * Nothing in this module may reconstruct an address.
 */
export interface LibrarySource {
	/** `library_source.id`. The chip's key. */
	id: number;
	/**
	 * What §17.8's cross-link needs: *"a degraded source on a library row links
	 * to that instance's Services row"*. It is the id the Services screen
	 * already anchors by (`#service-<id>`).
	 */
	serviceInstanceId: number;
	/** The chip's label. */
	serviceName: string;
	/** The icon, and the word §17.3 renders as the instance's kind. */
	serviceKind: string;
	/** One of `instance`, `root_folder`, `remote_library`, `tag`, `series_type`. */
	containerKind: string;
	/** The container the upstream itself reported, verbatim. */
	containerRef: string;
	/**
	 * The container's own name as the upstream reported it at bind time.
	 * ⚠️ GENUINELY OPTIONAL — the server omits the key rather than sending `""`,
	 * because a blank name and an unrecorded one are different facts.
	 */
	containerName?: string;
	/** §17.8's per-source metadata authority. On the wire even where §17.8
	 * suppresses the control, because the suppression is a rendering rule about a
	 * radio group with one option rather than a reason to withhold the fact. */
	isMetadataAuthority: boolean;
	/**
	 * RFC 3339 UTC: the upstream stopped reporting this container.
	 * ⚠️ NOTHING SETS IT. Present means the writer landed; absent means UNKNOWN.
	 */
	missingSince?: string;
}

/** One §17.8 row, as `libraryResponse` in `internal/httpapi/libraries.go` spells it. */
export interface Library {
	id: number;
	name: string;
	/**
	 * §17.8's URL identity, and the section is emphatic about how it is NOT
	 * rendered: *"The row's identifier is not rendered as a path … Drop the slash
	 * and the mono face"*. It is carried because it is the chip's `?lib=` value;
	 * the row view does not print it, and nothing here prefixes it.
	 */
	slug: string;
	/** `library.kind` verbatim, the schema's word. `kindLabel` maps it. */
	kind: string;
	/** Absent means ANY format, which is every row today. See the file header. */
	formats?: string[];
	sortOrder: number;
	enabled: boolean;
	includeInSearch: boolean;
	itemCount: number;
	/** ⚠️ NOTHING WRITES IT. See the file header. */
	orphanedAt?: string;
	/** Always an array. `[]` is §17.8's orphaned state, never "unknown". */
	sources: LibrarySource[];
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function num(value: unknown): number | undefined {
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function str(value: unknown): string {
	return typeof value === 'string' ? value : '';
}

function bool(value: unknown): boolean {
	return value === true;
}

/**
 * One source, or `undefined` for a frame this screen cannot draw.
 *
 * The id is the only required field: it is the chip's key, and a chip that
 * cannot be identified cannot be keyed in an `{#each}`. A missing service name
 * is NOT a reason to drop the source — dropping it would make a library look
 * more orphaned than it is, which is the one inference this screen must not
 * make.
 */
export function toLibrarySource(value: unknown): LibrarySource | undefined {
	if (!isRecord(value)) return undefined;
	const id = num(value.id);
	if (id === undefined) return undefined;
	const source: LibrarySource = {
		id,
		serviceInstanceId: num(value.service_instance_id) ?? 0,
		serviceName: str(value.service_name),
		serviceKind: str(value.service_kind),
		containerKind: str(value.container_kind),
		containerRef: str(value.container_ref),
		isMetadataAuthority: bool(value.is_metadata_authority)
	};
	// Assigned only when present, so an omitted optional key stays omitted rather
	// than becoming an `undefined`-valued one.
	const containerName = str(value.container_name);
	if (containerName !== '') source.containerName = containerName;
	const missingSince = str(value.missing_since);
	if (missingSince !== '') source.missingSince = missingSince;
	return source;
}

/**
 * `library.formats` — a JSON array over `edition.format`, forwarded verbatim by
 * the server as `json.RawMessage`.
 *
 * Anything that is not an array of non-empty strings yields `undefined`, which
 * is the same answer as the honest NULL: the server already drops and logs a
 * stored value that will not decode as an array of strings, so a shape arriving
 * here that is not one is a case neither side can render, and inventing a
 * format filter out of it would be worse than showing none.
 */
export function toFormats(value: unknown): string[] | undefined {
	if (!Array.isArray(value)) return undefined;
	const out = value.filter((f): f is string => typeof f === 'string' && f !== '');
	return out.length === 0 ? undefined : out;
}

/**
 * One library, or `undefined` for a frame this screen cannot draw.
 *
 * The id is the only required field: it is the row key. A missing name is NOT a
 * reason to drop the row — the cell has a rendering for an empty one, and
 * dropping it would make the table silently short by rows the server sent.
 */
export function toLibrary(value: unknown): Library | undefined {
	if (!isRecord(value)) return undefined;
	const id = num(value.id);
	if (id === undefined) return undefined;
	const library: Library = {
		id,
		name: str(value.name),
		slug: str(value.slug),
		kind: str(value.kind),
		sortOrder: num(value.sort_order) ?? 0,
		// `enabled` and `include_in_search` have no `omitempty`, so an absent key
		// is a broken frame rather than a fact. FALSE is the safe reading of a
		// broken frame in both cases: it understates what the library does rather
		// than claiming it is live and searched when nothing said so.
		enabled: bool(value.enabled),
		includeInSearch: bool(value.include_in_search),
		itemCount: num(value.item_count) ?? 0,
		// `sources` is always present and is `[]` rather than absent, because an
		// absent key reads as "unknown" and "this library has no sources" is
		// precisely what §17.8's orphaned state renders. A frame that omits it
		// anyway lands here as `[]`, which the state model reports as no sources
		// rather than passing an undefined array to the renderer.
		sources: Array.isArray(value.sources)
			? value.sources.map(toLibrarySource).filter((s): s is LibrarySource => s !== undefined)
			: []
	};
	const formats = toFormats(value.formats);
	if (formats !== undefined) library.formats = formats;
	const orphanedAt = str(value.orphaned_at);
	if (orphanedAt !== '') library.orphanedAt = orphanedAt;
	return library;
}

/**
 * THE LIST, IN THE SERVER'S OWN ORDER.
 *
 * `internal/store/libraries.go` ends its statement `ORDER BY l.sort_order,
 * l.name`, and this deliberately does not re-sort. A second implementation of an
 * ordering rule is a second thing that can disagree with the first, and the two
 * would disagree on the tie-break the moment one of them reached for
 * `localeCompare`: the column is plain `TEXT`, so SQLite compares it with the
 * BINARY collation, and `'Z'` sorts before `'a'` there and after it in every
 * locale. `sort_order` travels anyway, because §2.2 serves it so a client that
 * DOES re-sort locally can still send back a correct reorder — there is nothing
 * to send it to yet.
 *
 * A payload that is not the documented shape yields `[]`, which the screen
 * renders as its zero state. That is the same answer as an empty install, and it
 * is the right one: §2.2 guarantees `items` is present and is `[]` on an empty
 * install precisely so the two are NOT distinguishable, and a client that
 * invented a third reading would be inventing.
 */
export function toLibraries(payload: unknown): Library[] {
	if (!isRecord(payload)) return [];
	if (!Array.isArray(payload.items)) return [];
	return payload.items.map(toLibrary).filter((l): l is Library => l !== undefined);
}

/** The list. A local SQLite read behind the endpoint: it makes no upstream call
 * and is never on a path that waits for a service. No query parameters and no
 * paging — §2.1: a user's libraries are a set they created by hand. */
export async function fetchLibraries(): Promise<Library[]> {
	return toLibraries(await getJson(LIBRARIES_URL));
}

/* ── 2. the Kind column's vocabulary ──────────────────────────────────────── */

/**
 * §17.8: *"Label it `Movies · TV · Music · Books · Comics`, let the schema value
 * be the value"*, and *"The list's `Kind` column follows the same labels"*.
 *
 * ⚠️ THE SCHEMA'S ENUM IS WIDER THAN THE FIVE LABELS, AND THE GAP IS NOT AN
 * OVERSIGHT TO PAPER OVER. `library.kind` is
 * `CHECK (kind IN ('movie','series','artist','album','book','comic','game'))`
 * (`internal/db/migrations/00005_library_sync.sql`), which is seven; §17.8 names
 * five, and the two it does not name — `album` and `game` — have no product word
 * yet. `kindLabel` renders those VERBATIM rather than mapping them to a
 * neighbour or replacing them with a nothing-word, on `$lib/library`'s reasoning
 * for the same situation: a row that says `game` is the honest rendering of an
 * enum that grew, and a guess is a wrong label the user cannot see through.
 *
 * `person` is deliberately absent from the schema itself — a library of authors
 * is not a thing (ADR-0033) — so there is nothing here to decline.
 */
const KIND_LABEL: Record<string, string> = {
	movie: 'Movies',
	series: 'TV',
	artist: 'Music',
	book: 'Books',
	comic: 'Comics'
};

/** The Kind cell's text: the product's word where §17.8 gives one, else the
 * schema's own value. */
export function kindLabel(kind: string): string {
	return KIND_LABEL[kind] ?? kind;
}

/* ── 3. the Items column ──────────────────────────────────────────────────── */

const COUNT = new Intl.NumberFormat('en-GB');

/**
 * `item_count`, grouped.
 *
 * ONE STRING, NOT §9.1's TWO SLOTS, and that is the rule rather than a shortcut.
 * `app.css` carries exactly one `.unit` modifier, `.unit--size`, and its own
 * comment scopes the reserved unit box to SIZE COLUMNS: a reserved box costs
 * column width, and it names Home's `Items` at 107.375 px as one of the two
 * columns measured wrapping when the treatment was applied uniformly. So the
 * count is the whole cell and the header carries the noun.
 */
export function itemCountText(library: Library): string {
	return COUNT.format(library.itemCount);
}

/* ── 4. the source chips ──────────────────────────────────────────────────── */

/** One rendered chip. `missing` is the ONLY health signal, and it is one-way. */
export interface SourceChip {
	key: string;
	/** The instance's name, or the schema's fallback when the row carries none. */
	label: string;
	/** The full text for the `title` companion to `.trunc` (§9.1 tier 1). */
	title: string;
	/** The Services row this chip links to. */
	serviceInstanceId: number;
	/**
	 * ⚠️ TRUE MEANS `missing_since` WAS SET. FALSE MEANS NOBODY HAS SAID. It is
	 * never rendered as a positive health claim; see `libraryStates`.
	 */
	missing: boolean;
}

/**
 * A source with no `service_name` still gets a chip.
 *
 * The alternative is dropping it, and dropping a source makes a library read as
 * more orphaned than it is — the one inference §17.8's orphaned state must not
 * be reached by. `NOTHING.empty` is not right either: it is the word for a cell
 * with no value, and this is a source that exists whose name did not travel.
 */
const UNNAMED_SOURCE = 'Unnamed service';

function chipTitle(source: LibrarySource, label: string): string {
	// §17.8's *"upstream's own name beneath it, greyed and non-editable"*, folded
	// into the row's `title` because the row view has no second line for it. The
	// container ref is NOT appended: it is a Kavita library id in v0.1, machine
	// data with no meaning to a reader, and §9.1 keeps machine strings out of a
	// cell's identity text.
	return source.containerName === undefined ? label : `${label} · ${source.containerName}`;
}

/**
 * The Sources cell, capped.
 *
 * §9.1 caps a cell that renders one chip per related object at three plus
 * `+N more`; the live case §9.1 names is one Audiobookshelf feeding fifteen
 * libraries, and the reverse — one library over many instances — is §17.8's own
 * two-Radarr example. A source carrying `missing_since` is hoisted in front of
 * the cap, because a cell that hides the one broken source behind `+2 more` has
 * dropped the only fact on the row worth acting on.
 */
export function sourceChips(
	library: Library,
	max = 3
): { shown: SourceChip[]; more: number; total: number } {
	const chips = library.sources.map((source): SourceChip => {
		const label = source.serviceName === '' ? UNNAMED_SOURCE : source.serviceName;
		return {
			key: String(source.id),
			label,
			title: chipTitle(source, label),
			serviceInstanceId: source.serviceInstanceId,
			missing: source.missingSince !== undefined
		};
	});
	// A stable partition rather than a sort: the server's order is preserved
	// inside each half, so the cell does not reshuffle between renders.
	const ordered = [...chips.filter((c) => c.missing), ...chips.filter((c) => !c.missing)];
	if (ordered.length <= max) return { shown: ordered, more: 0, total: ordered.length };
	return { shown: ordered.slice(0, max), more: ordered.length - max, total: ordered.length };
}

/* ── 5. the State column ──────────────────────────────────────────────────── */

/** The status roles `app.css` exposes as `.st--warn` and `.st--none`.
 *
 * ⚠️ THERE IS NO `ok` ARM AND THERE CANNOT BE ONE. `.st--ok` is a claim that
 * something was measured and passed, and nothing on this wire measures a
 * library: `missing_since` has no writer, so its absence is silence. A green
 * tick here would be the writer's silence rendered as an observation. */
export type LibraryTone = 'warn' | 'none';

/** One state mark on a row. A row can carry several; none of them is positive. */
export interface LibraryStateMark {
	key: string;
	/** UsArr's own words. §17.3's split between the State and Problem columns
	 * applies here too: the schema's vocabulary does not belong in this cell. */
	word: string;
	tone: LibraryTone;
	/** An RFC 3339 stamp that qualifies the word, when the wire carried one.
	 * The screen formats it; this module does not own a clock. */
	at?: string;
}

/**
 * WHAT A ROW SAYS ABOUT ITSELF, AND EVERY ARM IS AN OBSERVATION.
 *
 * §17.8 names eight per-library states — importing, one source degraded, all
 * sources down, sources healthy with zero items, orphaned, no sink, needs
 * re-identification, and no change feed. **Four of those are claims about a
 * service's health, which is not on this wire at all**: this endpoint reads
 * `library`, `library_source` and `library_member`, and the only column in reach
 * that speaks to health is `missing_since`, which nothing sets. So this function
 * emits the states that ARE observable and stays silent on the rest, rather than
 * deriving a health word from a field that has no writer.
 *
 * ⚠️ AND IT NEVER EMITS A POSITIVE ONE. A row with nothing to say returns `[]`,
 * which the screen renders as §9.1's `—`. The tempting bug is the tick: with
 * `missing_since` absent on every source of every row today, an `ok` arm would
 * fire on every library in the product and read as "checked, and fine" while
 * nothing has ever checked anything.
 *
 * The array is ordered worst-first, so a truncating cell keeps the actionable
 * mark. Nothing is dropped: a row can carry several and all of them render.
 */
export function libraryStates(library: Library): LibraryStateMark[] {
	const marks: LibraryStateMark[] = [];

	if (library.sources.length === 0) {
		// §17.8's orphaned state, and it renders from the ARRAY rather than from
		// `orphaned_at`: `sources: []` is served unconditionally and is an
		// observation, while the timestamp has no writer. When something does
		// start writing it, it arrives here as the qualifier it was always meant
		// to be — §6.5 rule 5's *"shown with its reason"* — without this arm
		// changing. Until then the row says what is true and does not say when.
		const mark: LibraryStateMark = { key: 'no-sources', word: 'No sources', tone: 'warn' };
		if (library.orphanedAt !== undefined) mark.at = library.orphanedAt;
		marks.push(mark);
	} else {
		const missing = library.sources.filter((s) => s.missingSince !== undefined);
		if (missing.length > 0) {
			// The earliest stamp, because it answers "since when has this been
			// wrong", which is the question a stale replica raises. §9.1 bans a
			// parenthesised plural where the count is known, and it is known here.
			const at = missing
				.map((s) => s.missingSince ?? '')
				.reduce((earliest, next) => (next < earliest ? next : earliest));
			marks.push({
				key: 'source-missing',
				word:
					missing.length === 1 ? 'A source is missing' : `${missing.length} sources are missing`,
				tone: 'warn',
				at
			});
		}
		if (library.itemCount === 0) {
			// §17.8's *"sources healthy, zero items"*, with the half this wire
			// cannot support left off. The section's own example sentence is
			// *"Radarr is connected and reports 0 films"*, and `connected` is
			// exactly the word nothing here has measured, so the mark says only
			// what the count says.
			marks.push({ key: 'no-items', word: 'No items', tone: 'none' });
		}
	}

	// §17.8's detail view groups both of these under *Visibility*. They are read
	// from their own columns and are independent of each other, so both can
	// render on one row. Grey is the right role: neither is broken, and painting
	// a deliberate setting a warning colour makes the two states that ARE broken
	// harder to find (app.css, `.st--none`).
	if (!library.enabled) marks.push({ key: 'disabled', word: 'Turned off', tone: 'none' });
	if (!library.includeInSearch) {
		marks.push({ key: 'not-in-search', word: 'Not in search', tone: 'none' });
	}

	return marks;
}

/* ── 6. the three ways this screen has nothing to show ────────────────────── */

/**
 * A FAILED READ, AN ENDED SESSION AND AN EMPTY INSTALL ARE THREE DIFFERENT
 * THINGS, AND THIS TYPE IS WHY THEY READ DIFFERENTLY.
 *
 * The empty install is not in this union on purpose: it is not a failure, it is
 * a successful read of nothing, and it is the list's own `empty` state
 * (DESIGN-DIRECTION §10). What is here is the two ways the read did not happen.
 *
 * §2.6 gives the endpoint exactly two error statuses — `401 unauthorized` and
 * `500 internal` — and there is no `400`, because the endpoint takes no input.
 * So the split below is not a taxonomy invented for the screen; it is the
 * server's own, plus the transport failure that produces neither.
 */
export type LibrariesFailure =
	| {
			k: 'session';
			title: string;
			text: string;
	  }
	| {
			k: 'failed';
			title: string;
			text: string;
			/** The server's own words, rendered verbatim in `.verbatim` (§10). */
			verbatim: string;
	  };

/**
 * Which of the two happened.
 *
 * The 401 is told apart on the STATUS and on the server's `error` code, never on
 * the prose. It is a PROMPT rather than an error: nothing is broken, nothing was
 * lost, and there is no upstream text to quote because the server did not fail.
 *
 * ⚠️ AND THE SHELL, NOT THIS SCREEN, IS WHAT A USER ACTUALLY MEETS ON A 401.
 * `$lib/api` hands every 401 to `onUnauthorized`, which clears the session, and
 * `+layout.svelte` then renders its own `Not signed in. Taking you to the
 * sign-in page.` in place of this route's children before navigating. Driven in
 * Chromium against the built SPA with a late 401 and the /login route chunk
 * delayed, the screen's own session banner appeared in 0 of 400 polled frames.
 * This arm is therefore not a frame anyone has been shown, and it is not written
 * down here as one. It exists so that the OTHER arm cannot claim a 401 was a
 * failed local read and offer a Try again that can never succeed: a taxonomy is
 * worth having where its branches are mutually exclusive, not only where every
 * branch paints.
 *
 * Everything else is the failure arm, including a transport error, which
 * `$lib/api` wraps as an ApiError with status 0.
 */
export function describeFailure(error: unknown): LibrariesFailure {
	if (error instanceof ApiError && (error.status === 401 || error.code === 'unauthorized')) {
		return {
			k: 'session',
			title: 'Your session has ended',
			text: 'Sign in again and this screen will read your libraries. Nothing was lost.'
		};
	}
	return {
		k: 'failed',
		title: 'Your libraries could not be read',
		// The endpoint reads local SQLite, so the sentence points at UsArr rather
		// than at a service the user might go and restart. §13: every error string
		// names the component, the observed symptom and the next action.
		text: 'This list comes from UsArr’s own database and not from a service, so a service being down is not the cause. Try again, and if it keeps failing the server log has the query.',
		verbatim: error instanceof ApiError ? error.detail : String(error)
	};
}
