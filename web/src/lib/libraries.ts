/**
 * THE LIBRARIES SCREEN, CLIENT SIDE — `GET /api/v1/libraries`, JOINED TO
 * `GET /api/v1/services/health`.
 *
 * TWO READS, BECAUSE §17.8's ROW VIEW ASKS FOR A FACT NEITHER ONE HOLDS ALONE.
 * The row view is *"name · kind · item count · source chips with per-source
 * health · … · state"*, and per-source health is not in `library`,
 * `library_source` or `library_member`: the one column in reach that speaks to
 * it is `missing_since`, which nothing writes. The measurement is on the health
 * endpoint, keyed by service instance, and §4 below carries the proof that the
 * two keys are the same column of the same table rather than the assertion.
 * BOTH ARE TIER 0 LOCAL READS, so the join blocks nothing upstream and neither
 * read blocks the other's render: libraries alone still draws the table, and
 * health alone draws nothing here at all.
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
 *     would be reporting the writer's silence as an observation. ⚠️ THE HEALTH
 *     JOIN DOES NOT CHANGE THIS. It measures a SERVICE, and this column is about
 *     a CONTAINER the service reports: a Kavita in perfect health can stop
 *     serving a library, so a `connected` verdict is not evidence about
 *     `missing_since` and the two render as the separate facts they are.
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
 * ⚠️ AND ONE FIELD DESCRIBES A STATE THAT IS REACHABLE AND WHOSE ABSENCE IS
 * STILL NOT A PASS. `items[].completeness` (§2.6) carries what the last import
 * measured about how much of a library's upstream container UsArr's credential
 * could see. It is ABSENT for every library nothing measured — every Kavita
 * library today — so absence is a fact about UsArr rather than about the
 * library, exactly as `missing_since`'s is. What is different is that when it IS
 * present it has three values rather than two, and the third, `unverified`,
 * RENDERS OUT LOUD: it is the state the whole check degrades into if the
 * upstream route it depends on is ever guarded, and a screen that drew it as
 * silence would report a service that had stopped being measurable as a service
 * with nothing wrong.
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

import { ApiError, getJson, type ServiceHealth } from './api';
import { applicationName, isIndexer } from './services';

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
	/**
	 * What the last import measured about how much of this library's upstream
	 * containers UsArr's credential could actually see.
	 *
	 * ⚠️ ABSENT MEANS NOTHING WAS MEASURED, NEVER "COMPLETE". It is absent for
	 * every library whose source runs no completeness check — today that is every
	 * Kavita library — and for every library that has never been imported. It is
	 * the one optional key on this wire whose absence is a fact about UsArr
	 * rather than about the library, and `libraryStates` below emits nothing at
	 * all for it, which is the same treatment `missingSince` gets for the same
	 * reason.
	 */
	completeness?: LibraryCompleteness;
	/**
	 * What the last import recorded about items it READ in this library's
	 * containers and deliberately did not map.
	 *
	 * ⚠️ IT IS NOT `completeness` AND IT IS NOT ITS INVERSE. That one answers
	 * whether the credential SAW the whole container; this one answers whether
	 * UsArr MAPPED what it saw. They are independent, they fail independently,
	 * and `state: 'none'` here is not a statement that the library is complete.
	 *
	 * ⚠️ ABSENT MEANS NOTHING OBSERVED THIS LIBRARY, never "nothing was skipped".
	 * The server serves those as two different values on purpose, so this module
	 * must never collapse them.
	 */
	skipped?: LibrarySkips;
}

/**
 * ONE LIBRARY'S SKIP VERDICT, as `librarySkipsResponse` in
 * `internal/httpapi/libraries.go` spells it.
 *
 * # Why there are three readings and only two members
 *
 * Upstream writes a skip row for every container an import WALKED, zero or not,
 * so "the walk left nothing out" is a recorded zero and "nothing has ever
 * counted" is no record at all. The server serves the difference: `left_out`,
 * `none`, or the key absent. All three read differently here, which is the whole
 * reason the key exists.
 *
 * ⚠️ The server USED to write a row only where something was skipped and tell
 * the last two apart by cross-referencing a second record; ADR-0063 replaced
 * that with the zero row. Nothing on this side changed — the three values were
 * already three values — and that is the point: only the evidence moved.
 *
 * # And it is not a completeness check
 *
 * ⚠️ `none` MEANS THE ADAPTER RECORDED NOTHING LEFT OUT. It does not mean the
 * library is complete, it does not mean the credential saw the whole container,
 * and nothing in this module may phrase it as either. That is `completeness`'s
 * question and it has its own three states.
 */
export interface LibrarySkips {
	state: 'left_out' | 'none';
	/** ⚠️ ABSENT UNDER `none`: there is no count to serve there rather than a
	 * count of zero, which is the same treatment `completeness` gives its numbers
	 * under `unverified`. */
	items?: number;
	/** UsArr's own short sentence about why. Present only with `left_out`, and
	 * never upstream text. */
	reason?: string;
	/** RFC 3339 UTC: when the verdict was recorded. Under `none` it is the stamp
	 * of the observation the state rests on. */
	recordedAt?: string;
}

/**
 * ONE LIBRARY'S COMPLETENESS VERDICT, as `libraryCompletenessResponse` in
 * `internal/httpapi/libraries.go` spells it.
 *
 * # Why there are three states and not a boolean
 *
 * The measurement behind this is a subtraction between two counts an upstream
 * publishes: what UsArr's credential was shown, and what the container holds
 * with no content filter applied. On BookOrbit the second one comes from a route
 * that carries no permission guard TODAY — a property of somebody else's service
 * and not a promise to UsArr. If it is ever guarded, every probe refuses.
 *
 * A two-state design would report every library as complete on the day it
 * stopped being able to tell. So `unverified` is a first-class state, this
 * module never collapses it into either of the others, and the row it produces
 * says so in words rather than rendering as silence.
 *
 * # And it is a BOOK-LEVEL check
 *
 * ⚠️ `complete` MEANS EVERY ITEM IN THIS CONTAINER ARRIVED. It says nothing
 * about whether whole containers are hidden from UsArr's credential, which is a
 * different question and is not answerable from a read-only account at all — the
 * upstream returns the same refusal for a container that exists and one that
 * does not. Nothing in this module may phrase a verdict as a statement about the
 * service.
 */
export interface LibraryCompleteness {
	state: 'complete' | 'shortfall' | 'unverified';
	/**
	 * ⚠️ ALL THREE COUNTS ARE ABSENT UNDER `unverified`, and the server omits
	 * them rather than sending zeroes: in that state they are unknown, not small.
	 * They are also NOT `itemCount` — that one counts what UsArr wrote, these
	 * count what the upstream said it holds, and the two differ for reasons that
	 * have nothing to do with a filter.
	 */
	total?: number;
	visible?: number;
	hidden?: number;
	/** UsArr's own sentence about why the check could not be made. Present only
	 * with `unverified`, and never upstream text. */
	reason?: string;
	/** RFC 3339 UTC: when the verdict was recorded. It is what makes a shortfall
	 * read as a measurement rather than as a live fact. */
	checkedAt?: string;
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
	const completeness = toCompleteness(value.completeness);
	if (completeness !== undefined) library.completeness = completeness;
	const skipped = toSkips(value.skipped);
	if (skipped !== undefined) library.skipped = skipped;
	return library;
}

/**
 * One skip verdict, or `undefined` for anything this screen must not render.
 *
 * ⚠️ AN UNRECOGNISED `state` YIELDS `undefined`, WHICH READS AS "NOTHING WAS
 * OBSERVED". It is not mapped to the nearest member and it is certainly not
 * defaulted to `none`: a member written by some later adapter is a verdict this
 * build cannot read, and the only honest rendering of an unreadable record is no
 * record. `toCompleteness` refuses the same way and the server refuses on the
 * way out (`SkipStateOf`), which makes this the second of two arms — deliberate,
 * because the two sides are deployed independently.
 *
 * The count and the reason are read only under `left_out`. Under `none` the
 * server omits them; if one arrived anyway it is DROPPED rather than rendered,
 * because a count under that label is a claim the label does not make.
 */
export function toSkips(value: unknown): LibrarySkips | undefined {
	if (!isRecord(value)) return undefined;
	const state = str(value.state);
	if (state !== 'left_out' && state !== 'none') return undefined;

	const out: LibrarySkips = { state };
	if (state === 'left_out') {
		const items = num(value.items);
		if (items !== undefined) out.items = items;
		const reason = str(value.reason);
		if (reason !== '') out.reason = reason;
	}
	const recordedAt = str(value.recorded_at);
	if (recordedAt !== '') out.recordedAt = recordedAt;
	return out;
}

/**
 * One completeness verdict, or `undefined` for anything this screen must not
 * render.
 *
 * ⚠️ AN UNRECOGNISED `state` YIELDS `undefined`, WHICH RENDERS AS "NOTHING WAS
 * MEASURED". It is not mapped to the nearest member and it is certainly not
 * defaulted to `complete`: a fourth state written by some later adapter is a
 * verdict this build cannot read, and the only honest rendering of an unreadable
 * measurement is no measurement. The server refuses the same value on the way
 * out (`CompletenessStateOf`), so this arm is the second of two rather than the
 * only one — which is deliberate, because the two sides are deployed
 * independently.
 *
 * The counts are read only for the two measured states. Under `unverified` the
 * server omits them; if one arrived anyway it is DROPPED here rather than
 * rendered, because a number under that label is a claim the label denies.
 */
export function toCompleteness(value: unknown): LibraryCompleteness | undefined {
	if (!isRecord(value)) return undefined;
	const state = str(value.state);
	if (state !== 'complete' && state !== 'shortfall' && state !== 'unverified') return undefined;

	const out: LibraryCompleteness = { state };
	if (state === 'unverified') {
		const reason = str(value.reason);
		if (reason !== '') out.reason = reason;
	} else {
		const total = num(value.total_items);
		const visible = num(value.visible_items);
		const hidden = num(value.hidden_items);
		if (total !== undefined) out.total = total;
		if (visible !== undefined) out.visible = visible;
		if (hidden !== undefined) out.hidden = hidden;
	}
	const checkedAt = str(value.checked_at);
	if (checkedAt !== '') out.checkedAt = checkedAt;
	return out;
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

/* ── 4. the health join ───────────────────────────────────────────────────── */

/**
 * THE OTHER READ, AND THE KEY WAS CHECKED RATHER THAN ASSUMED.
 *
 * `GET /api/v1/libraries` carries no health at all: it reads `library`,
 * `library_source` and `library_member`, and the only column in reach that
 * speaks to a source's condition is `missing_since`, which nothing writes.
 * `GET /api/v1/services/health` carries the measurement, keyed by service
 * instance, and both are Tier 0 local reads, so joining them blocks nothing
 * upstream (principle 1).
 *
 * THE KEY IS `service_instance.id`, ON BOTH SIDES, AND HERE IS THE PROOF RATHER
 * THAN THE ASSERTION:
 *
 *   · `sources[].service_instance_id` is `library_source.service_instance_id`,
 *     declared `INTEGER NOT NULL REFERENCES service_instance(id) ON DELETE
 *     CASCADE` (internal/db/migrations/00005_library_sync.sql:637) and read
 *     through `JOIN service_instance si ON si.id = ls.service_instance_id`
 *     (internal/store/libraries.go:344).
 *   · `services[].id` is `serviceHealthResponse.ID`, assigned `si.ID` in
 *     `healthRow` (internal/httpapi/services.go:714) off the row
 *     `ListServiceInstances` scans from `service_instance`'s own `id` column
 *     (internal/store/serviceinstance.go:68).
 *
 * Same table, same primary key, so the two join. ⚠️ THEY ARE STILL TWO READS AT
 * TWO MOMENTS, and both directions of disagreement are real rather than
 * theoretical: the source read filters `si.deleted_at IS NULL` and applies the
 * caller's scope, so an instance that is soft-deleted or scoped away between the
 * two requests can be named by one and absent from the other. Every function
 * below is written against a join that can miss in either direction, and neither
 * miss is ever read as health.
 */
export type HealthRead =
	{ k: 'unread' } | { k: 'read'; byInstance: ReadonlyMap<number, ServiceHealth> };

/**
 * Health has not answered, or answered with a failure.
 *
 * ⚠️ IT IS NOT "NOTHING IS WRONG". Every health-derived rendering below is
 * suppressed on this value rather than defaulted, so the screen says the same
 * things it said before the join existed, plus one sentence above the table
 * saying that health is what is missing.
 */
export const HEALTH_UNREAD: HealthRead = { k: 'unread' };

/** The health rows, indexed by the id both wires share. */
export function healthRead(rows: readonly ServiceHealth[]): HealthRead {
	const byInstance = new Map<number, ServiceHealth>();
	for (const row of rows) byInstance.set(row.id, row);
	return { k: 'read', byInstance };
}

/**
 * What the health read says about ONE source's instance.
 *
 * `connected` is the only affirmative member and it is the only one that
 * requires a measurement: `state === 'healthy'` AND `stale === false`, which
 * together mean a probe ran and passed. `stale` outranks `healthy` for the
 * reason the Services screen states at `stateLabel` — the server answers
 * `healthy` on a stored `last_ok_at` alone, so an instance that answered once
 * when it was added arrives as `healthy`, `stale: true`, and that is a fact
 * about the past.
 *
 * The two silences are kept apart on purpose, because they have different
 * causes and different fixes: `unlisted` is a source naming an instance the
 * health read does not mention, `unchecked` is an instance it mentions and has
 * not measured.
 */
export type SourceState =
	'connected' | 'degraded' | 'down' | 'reidentify' | 'off' | 'unchecked' | 'unlisted' | 'unread';

export interface SourceVerdict {
	state: SourceState;
	/** The word the chip prints beside the name. '' only while health is unread. */
	word: string;
	tone: LibraryTone;
}

/**
 * ONE INSTANCE'S CONDITION, IN THIS SCREEN'S WORDS.
 *
 * ⚠️ THE PRECEDENCE IS THE SERVICES SCREEN'S, ARM FOR ARM — re-identification,
 * then turned off, then the breaker, then down, then degraded, then stale, then
 * healthy — and `services.test.ts`'s neighbour test holds the two together: this
 * function answers `connected` exactly where `stateLabel` answers tone `ok`. Two
 * screens rendering the same instance from the same row must not disagree about
 * it, and a second implementation of a precedence is the ordinary way they come
 * to.
 *
 * The words are not `stateLabel`'s, and that is deliberate: this is a chip
 * beside a library, not the column a user scans for what is broken, so it says
 * the short form and links to the row that says the long one.
 */
export function sourceVerdict(source: LibrarySource, read: HealthRead): SourceVerdict {
	if (read.k === 'unread') return { state: 'unread', word: '', tone: 'none' };

	const h = read.byInstance.get(source.serviceInstanceId);
	// ABSENT IS NOT HEALTHY. The health read did not mention this instance, so
	// nothing is known about it and the chip says exactly that.
	if (h === undefined) return { state: 'unlisted', word: 'not listed', tone: 'none' };

	if (h.state === 'needs re-identification') {
		return {
			state: 'reidentify',
			word: `may be a different ${applicationName(h.kind)}`,
			tone: 'err'
		};
	}
	if (!h.enabled) return { state: 'off', word: 'turned off', tone: 'none' };
	// An open breaker and a `down` state are one answer here: UsArr is not
	// getting through. The Services row keeps the distinction between the two,
	// which is what the chip links to.
	if (h.breakerState.toLowerCase() === 'open' || h.state === 'down') {
		return { state: 'down', word: 'not answering', tone: 'err' };
	}
	if (h.state === 'degraded') return { state: 'degraded', word: 'failing', tone: 'warn' };
	if (h.stale) return { state: 'unchecked', word: 'not checked', tone: 'none' };
	if (h.state === 'healthy') return { state: 'connected', word: 'connected', tone: 'none' };
	// `unknown` with a probe behind it: the probe ran and could not classify the
	// instance. An honest "not checked" still beats a claim.
	return { state: 'unchecked', word: 'not checked', tone: 'none' };
}

/**
 * Whether this instance's catalogue counts may be read at all.
 *
 * An indexer contributes no works and syncs no catalogue, so its `null` / `0`
 * pair is `Not applicable` rather than `never synced` (http-api.md §3.2). No
 * v0.1 source is an indexer — §17.8 proposes no library for Prowlarr — and the
 * guard is here so that the day one is, the row does not accuse it of never
 * having imported a catalogue it does not have.
 */
function catalogueHealth(source: LibrarySource, read: HealthRead): ServiceHealth | undefined {
	if (read.k === 'unread') return undefined;
	const h = read.byInstance.get(source.serviceInstanceId);
	if (h === undefined || isIndexer(h)) return undefined;
	return h;
}

/**
 * THE REVERSE DIRECTION OF THE SAME DISAGREEMENT: a health row no library names.
 *
 * It is not a library, so it is not a row on this screen and nothing here
 * invents one. It IS worth one sentence above the table, because on the shipped
 * binary a library appears when a connected service finishes its first import,
 * so a catalogue service feeding nothing is the answer to "why is this screen
 * empty". Indexers are excluded by construction rather than by wording: §17.8
 * proposes no library for Prowlarr, so a Prowlarr feeding none is not news.
 */
export function unboundServices(libraries: readonly Library[], read: HealthRead): ServiceHealth[] {
	if (read.k === 'unread') return [];
	const bound = new Set<number>();
	for (const library of libraries) {
		for (const source of library.sources) bound.add(source.serviceInstanceId);
	}
	const out: ServiceHealth[] = [];
	for (const h of read.byInstance.values()) {
		if (!bound.has(h.id) && !isIndexer(h)) out.push(h);
	}
	return out;
}

/**
 * The two sentences that sit above the table, both of them §9.1's rule that a
 * fact identical on every row is stated once and its column dropped.
 *
 * The unread arm is the important one: it is what keeps the table honest when
 * health fails and libraries succeeds. The table still draws, every row still
 * renders from the replica, and no chip says `connected`, so the sentence has to
 * say which of those three is the reason.
 */
export function healthNote(read: HealthRead): string {
	if (read.k === 'unread') {
		return 'Service health could not be read, so no source below says whether it is connected. The libraries themselves come from UsArr’s own database and are complete.';
	}
	return 'Each source carries what the Services screen last measured about it. A source that read does not mention says so, rather than being counted as connected.';
}

/**
 * THE SENTENCE ABOVE THE TABLE FOR THE SHORTFALL, and the reason it is a
 * sentence rather than a longer cell.
 *
 * §9.1: a fact identical on every affected row is stated ONCE above the table.
 * The per-row mark carries the NUMBERS, which differ per library; this carries
 * the FIX, which does not — the filter is on the service account, not in UsArr,
 * so no control on this screen can change it and the copy has to say where to
 * go. §13's rule for an error string applies to this one too: name the
 * component, the observed symptom, and the next action.
 *
 * The two arms are independent and both can render. `unverified` is deliberately
 * NOT phrased as a failure: nothing is broken, a measurement was not available,
 * and the sentence exists so an absent warning is never read as a pass.
 */
export function completenessNote(libraries: readonly Library[]): string {
	const short = libraries.filter((l) => l.completeness?.state === 'shortfall').length;
	const unknown = libraries.filter((l) => l.completeness?.state === 'unverified').length;

	const parts: string[] = [];
	if (short > 0) {
		parts.push(
			`A content filter on the account UsArr connects with is hiding books from ${short === 1 ? 'a library' : `${short} libraries`}. The filter is set on that service, not in UsArr: widen it there and the next import picks the books up.`
		);
	}
	if (unknown > 0) {
		parts.push(
			`UsArr could not check ${unknown === 1 ? 'one library' : `${unknown} libraries`} for hidden books. ${unknown === 1 ? 'It says so' : 'They say so'} below rather than being counted as complete.`
		);
	}
	return parts.join(' ');
}

/**
 * THE SENTENCE ABOVE THE TABLE FOR WHAT THE IMPORT LEFT OUT, and the second half
 * is the load-bearing one.
 *
 * §9.1 again: the per-row mark carries the count, which differs per library;
 * this carries what the count MEANS, which does not. And what it means has a
 * boundary — ⚠️ **a skip count is not a completeness check**. It says how many
 * items were read and not mapped; it says nothing about whether the items that
 * were mapped are all there are. Letting one clean number read as "this library
 * is complete" is the exact failure ADR-0061 exists to prevent, and this is its
 * sibling axis, so the sentence says so out loud rather than leaving it to a doc
 * comment.
 *
 * ⚠️ THE SECOND ARM IS GATED ON THE FIRST, AND THAT IS A DECISION. "N libraries
 * are not counted this way at all" is true on every Kavita-only install, for
 * every row, forever — §17.4 rule 5's definition of noise. It becomes news only
 * once the screen has told the user that items get left out, because then the
 * scope of that telling matters: the reader needs to know which rows the claim
 * covers. So it renders beside the first sentence and never alone.
 */
export function skippedNote(libraries: readonly Library[]): string {
	const left = libraries.filter((l) => l.skipped?.state === 'left_out');
	if (left.length === 0) return '';

	const items = left.reduce((n, l) => n + (l.skipped?.items ?? 0), 0);
	const parts = [
		`UsArr read ${COUNT.format(items)} ${items === 1 ? 'item' : 'items'} in ${
			left.length === 1 ? 'a library' : `${left.length} libraries`
		} and did not map ${items === 1 ? 'it' : 'them'}, so ${
			left.length === 1 ? 'that library’s' : 'those libraries’'
		} count is short by that many. It is not a completeness check: it says what was left out, not that the rest arrived.`
	];

	const unobserved = libraries.filter((l) => l.skipped === undefined).length;
	if (unobserved > 0) {
		parts.push(
			`Nothing counted this for ${
				unobserved === 1 ? 'one other library' : `${unobserved} other libraries`
			}, so ${unobserved === 1 ? 'its row says' : 'their rows say'} nothing either way.`
		);
	}
	return parts.join(' ');
}

/** `''` when every service feeds a library, which is the ordinary case. */
export function unboundNote(services: readonly ServiceHealth[]): string {
	if (services.length === 0) return '';
	if (services.length === 1) return `${services[0].name} feeds no library yet.`;
	return `${services.length} services feed no library yet.`;
}

/* ── 5. the source chips ──────────────────────────────────────────────────── */

/** One rendered chip: the instance, and what health last said about it. */
export interface SourceChip {
	key: string;
	/** The instance's name, or the schema's fallback when the row carries none. */
	label: string;
	/** The full text for the `title` companion to `.trunc` (§9.1 tier 1). */
	title: string;
	/**
	 * WHAT THE CHIP PRINTS BESIDE THE NAME, WHICH IS NOT ALWAYS THE VERDICT'S
	 * WORD. The chip has one colour and must not disagree with its own text: a
	 * source whose container stopped being reported is amber on `missing` alone,
	 * so an amber chip reading `connected` would paint one fact and name another.
	 * The rule is that the word matches the colour — the verdict wins whenever it
	 * has something alarming to say, and `missing` supplies the word when it does
	 * not. Both facts always survive on `title`.
	 */
	word: string;
	/** The Services row this chip links to. */
	serviceInstanceId: number;
	/**
	 * ⚠️ TRUE MEANS `missing_since` WAS SET. FALSE MEANS NOBODY HAS SAID. It is a
	 * fact about the CONTAINER, not about the service, and it is independent of
	 * `verdict`: an upstream in perfect health can stop reporting a library.
	 */
	missing: boolean;
	/** What health said about this source's instance. */
	verdict: SourceVerdict;
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

/** The container's own name, and now the health word, folded into `title`. */
function chipTitle(source: LibrarySource, label: string, verdict: SourceVerdict): string {
	// §17.8's *"upstream's own name beneath it, greyed and non-editable"*, folded
	// into the row's `title` because the row view has no second line for it. The
	// container ref is NOT appended: it is a Kavita library id in v0.1, machine
	// data with no meaning to a reader, and §9.1 keeps machine strings out of a
	// cell's identity text.
	const parts = [label];
	if (source.containerName !== undefined) parts.push(source.containerName);
	if (verdict.word !== '') parts.push(verdict.word);
	if (source.missingSince !== undefined) parts.push('no longer reported');
	return parts.join(' · ');
}

/**
 * Worst first, so the cap below never hides the source worth acting on.
 *
 * A rank rather than a sort key on the tone alone, because `unlisted`, `off` and
 * `unchecked` share `none` with `connected` and are not the same news: a silence
 * a user might act on outranks a measurement that passed.
 */
function chipRank(chip: SourceChip): number {
	if (chip.verdict.tone === 'err') return 0;
	if (chip.verdict.tone === 'warn' || chip.missing) return 1;
	if (chip.verdict.state === 'connected' || chip.verdict.state === 'unread') return 3;
	return 2;
}

/**
 * The Sources cell, capped.
 *
 * §9.1 caps a cell that renders one chip per related object at three plus
 * `+N more`; the live case §9.1 names is one Audiobookshelf feeding fifteen
 * libraries, and the reverse — one library over many instances — is §17.8's own
 * two-Radarr example. A source in trouble is hoisted in front of the cap,
 * because a cell that hides the one broken source behind `+2 more` has dropped
 * the only fact on the row worth acting on.
 */
export function sourceChips(
	library: Library,
	read: HealthRead,
	max = 3
): { shown: SourceChip[]; more: number; total: number } {
	const chips = library.sources.map((source): SourceChip => {
		const label = source.serviceName === '' ? UNNAMED_SOURCE : source.serviceName;
		const verdict = sourceVerdict(source, read);
		const missing = source.missingSince !== undefined;
		return {
			key: String(source.id),
			label,
			word: missing && verdict.tone === 'none' ? 'no longer reported' : verdict.word,
			title: chipTitle(source, label, verdict),
			serviceInstanceId: source.serviceInstanceId,
			missing,
			verdict
		};
	});
	// A stable partition rather than a sort: the server's order is preserved
	// inside each rank, so the cell does not reshuffle between renders.
	const ordered = [0, 1, 2, 3].flatMap((rank) => chips.filter((c) => chipRank(c) === rank));
	if (ordered.length <= max) return { shown: ordered, more: 0, total: ordered.length };
	return { shown: ordered.slice(0, max), more: ordered.length - max, total: ordered.length };
}

/* ── 6. the State column ──────────────────────────────────────────────────── */

/** The status roles `app.css` exposes as `.st--err`, `.st--warn` and `.st--none`.
 *
 * ⚠️ THERE IS STILL NO `ok` ARM, AND THE HEALTH JOIN DID NOT EARN ONE. `.st--ok`
 * on this screen would be a claim that THIS LIBRARY was measured and passed, and
 * what the join measures is a SERVICE: a healthy Kavita says nothing about
 * whether this library's membership is current, and the two columns that would
 * say so — `missing_since` and `orphaned_at` — still have no writer anywhere in
 * the tree (http-api.md §2.4). A green tick would also fire on nearly every row,
 * which is §17.4 rule 5 twice over.
 *
 * ⚠️ `err` IS NEW AND IS NOT A WEAKENING OF THAT. It arrived with the join,
 * because the join brought states that ARE errors — an instance that is not
 * answering, or whose identity is in doubt — and `.st--err` is the role the
 * Services screen already paints those with. Painting the same instance amber on
 * one screen and red on the other would be two screens disagreeing about one
 * row. The invariant that survives is the one that was always the point: nothing
 * on this screen renders a positive health claim. */
export type LibraryTone = 'err' | 'warn' | 'none';

/** One state mark on a row. A row can carry several; none of them is positive. */
export interface LibraryStateMark {
	key: string;
	/** UsArr's own words. §17.3's split between the State and Problem columns
	 * applies here too: the schema's vocabulary does not belong in this cell. */
	word: string;
	tone: LibraryTone;
	/** The qualifier under the word, or '' when the word says everything. §17.8
	 * asks the degraded state to NAME the source, and this is where the name
	 * goes when there is exactly one to name. */
	detail?: string;
	/** An RFC 3339 stamp that qualifies the word, when the wire carried one.
	 * The screen formats it; this module does not own a clock. */
	at?: string;
}

function plural(n: number, one: string, many: string): string {
	return n === 1 ? one : `${n} ${many}`;
}

/**
 * WHAT A ROW SAYS ABOUT ITSELF, AND EVERY ARM IS AN OBSERVATION.
 *
 * §17.8 names eight per-library states — importing, one source degraded, all
 * sources down, sources healthy with zero items, orphaned, no sink, needs
 * re-identification, and no change feed. FIVE OF THOSE ARE NOW OBSERVABLE,
 * because the health read supplies the half `GET /api/v1/libraries` never
 * carried, and the other three are still not, for reasons that are about the
 * wire rather than about this function:
 *
 *   · NO SINK is a screen-level fact, not a row-level one: no service v0.1
 *     connects can be a request destination at all, so §17.8 drops the column
 *     and the screen says so once in its own copy. Nothing for a mark to do.
 *   · NO CHANGE FEED needs the sync CHANNEL, and no endpoint publishes one. The
 *     health row says WHEN a full import last completed, never by which channel
 *     (`$lib/services`, `describeSync`). §17.8 also measures that no v0.1 source
 *     is in this state.
 *   · IMPORTING needs an import IN FLIGHT, and NEITHER OF THIS SCREEN'S TWO
 *     READS CARRIES ONE. Both are point-in-time SQLite reads: the health row
 *     says when an import last COMPLETED, and no field on either wire says one
 *     is running. ⚠️ THAT IS A STATEMENT ABOUT THESE TWO ENDPOINTS AND NOT ABOUT
 *     THE PRODUCT, and the difference is new as of `POST
 *     /api/v1/services/{id}/sync` (internal/httpapi/imports.go): the in-flight
 *     fact now exists in two places this screen does not look — a 409
 *     `import_in_progress` on that endpoint, which is a WRITE and cannot be on a
 *     render path, and the `import.progress` frame on `GET /api/events`
 *     (internal/httpapi/events.go), which is a live subscription this screen
 *     does not hold. Whether the Libraries screen should hold one is a design
 *     question rather than a join, so nothing here guesses at it.
 *
 *     What the two reads DO carry is the aftermath — `last_full_sync_at: null`
 *     with `work_count > 0`, which http-api.md §3.2 names "a partial import's
 *     rows stand" — and the mark below is named after THAT, an import that did
 *     not finish, rather than borrowed from §17.8's word for a live one. An
 *     unfinished import and a running one are not the same claim, and the same
 *     pair is left behind by an import that failed.
 *
 * ⚠️ AND IT STILL NEVER EMITS A POSITIVE ONE. A row with nothing to say returns
 * `[]`, which the screen renders as §9.1's `—`. The tempting bug is the tick:
 * health measures a service, this column describes a library, and the columns
 * that would let a library be pronounced fine have no writer.
 *
 * The array is ordered worst-first, so a truncating cell keeps the actionable
 * mark. Nothing is dropped: a row can carry several and all of them render.
 */
export function libraryStates(library: Library, read: HealthRead): LibraryStateMark[] {
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
		return withVisibility(library, marks);
	}

	const verdicts = library.sources.map((source) => ({
		source,
		verdict: sourceVerdict(source, read)
	}));
	const withState = (state: SourceState) => verdicts.filter((v) => v.verdict.state === state);
	const nameOf = (rows: { source: LibrarySource }[]): string | undefined =>
		rows.length === 1 && rows[0].source.serviceName !== '' ? rows[0].source.serviceName : undefined;

	// ── the health-derived arms, all of them silent while health is unread ──
	const reidentify = withState('reidentify');
	if (reidentify.length > 0) {
		// §17.8's *needs re-identification*: *"blocking banner, membership
		// recompute paused, because membership derived from an untrustworthy id
		// space is worse than stale membership"*. The detail is `sync is paused`
		// verbatim from `$lib/services`, so both screens say one thing about it.
		const app = applicationName(read.k === 'read' ? kindOf(reidentify[0].source, read) : '');
		marks.push({
			key: 'reidentify',
			word:
				reidentify.length === 1
					? `A source may be a different ${app}`
					: `${reidentify.length} sources may be different services`,
			tone: 'err',
			detail: 'sync is paused'
		});
	}

	const down = withState('down');
	if (down.length > 0 && down.length === verdicts.length) {
		// §17.8's *all sources down*, which the section calls *"the replica
		// principle's demo"*: every source is unreachable and the row, the counts
		// and the whole table are still here.
		marks.push({
			key: 'all-down',
			word: 'No source is answering',
			tone: 'err',
			detail: 'this list still renders from the replica'
		});
	} else if (down.length > 0) {
		marks.push({
			key: 'source-down',
			word: plural(down.length, 'A source is not answering', 'sources are not answering'),
			tone: 'err',
			...detailOf(nameOf(down))
		});
	}

	const degraded = withState('degraded');
	if (degraded.length > 0) {
		// §17.8's *one source degraded*, whose own instruction is that the banner
		// NAMES it and *"the grid does not grey out"*. The name is the detail when
		// there is one source to name; the chip in the Sources cell names it in
		// every case and links to the Services row.
		marks.push({
			key: 'source-degraded',
			word: plural(
				degraded.length,
				'A source is failing some attempts',
				'sources are failing some attempts'
			),
			tone: 'warn',
			...detailOf(nameOf(degraded))
		});
	}

	const missing = verdicts.filter((v) => v.source.missingSince !== undefined);
	if (missing.length > 0) {
		// The earliest stamp, because it answers "since when has this been
		// wrong", which is the question a stale replica raises. §9.1 bans a
		// parenthesised plural where the count is known, and it is known here.
		const at = missing
			.map((v) => v.source.missingSince ?? '')
			.reduce((earliest, next) => (next < earliest ? next : earliest));
		marks.push({
			key: 'source-missing',
			word: plural(missing.length, 'A source is missing', 'sources are missing'),
			tone: 'warn',
			at
		});
	}

	const unlisted = withState('unlisted');
	if (unlisted.length > 0) {
		// THE JOIN MISSED, AND THAT IS ITS OWN FACT RATHER THAN A HEALTH VERDICT.
		// The library names an instance the health read does not mention, which
		// the two statements make reachable: the source read filters
		// `si.deleted_at IS NULL` and both reads apply the caller's scope, so an
		// instance deleted or scoped away between the two requests lands here.
		// Grey, because nothing is broken: nothing was measured.
		marks.push({
			key: 'source-unlisted',
			// `Services` is the screen's own name, which is where the answer is.
			// The longer form `A source is not in the services list` wrapped the
			// cell to a second line at every width measured.
			word: plural(unlisted.length, 'A source is not in Services', 'sources are not in Services'),
			tone: 'none',
			...detailOf(nameOf(unlisted))
		});
	}

	// ── the catalogue-freshness arms, read as http-api.md §3.2's PAIR ──
	const catalogue = verdicts
		.map((v) => catalogueHealth(v.source, read))
		.filter((h): h is ServiceHealth => h !== undefined);
	const neverSynced = catalogue.filter((h) => h.lastFullSyncAt === null && h.workCount === 0);
	const partial = catalogue.filter((h) => h.lastFullSyncAt === null && h.workCount > 0);
	const connected = verdicts.filter((v) => v.verdict.state === 'connected');

	// ⚠️ THE THREE ARMS BELOW ARE ONE `else if` CHAIN, AND THE ORDER IS A DECISION
	// RATHER THAN THE SHAPE THE EDITS LEFT BEHIND. Both catalogue-freshness arms sit
	// AHEAD of `itemCount === 0`, so a library whose count is zero and one of whose
	// sources has never synced — or left a partial import behind — says only why the
	// catalogue is unfinished. Neither `Connected and empty` nor `No items` renders
	// beside it.
	//
	// That looks like this function's worst-first rule and is NOT it. The health arms
	// above accumulate because *nothing is dropped*: a source that is not answering
	// and a zero count are two facts, and neither explains the other. Here one DOES
	// explain the other, and the suppressed line is not merely lower down the cell,
	// it is FALSE. Measured by making the arm independent: two sources, zero items,
	// the second never synced then renders `A source has never synced` AND
	// `Connected and empty · their last imports finished` on one row. `connected`
	// only asks `state === 'healthy'` (`sourceVerdict`), so every source can be
	// connected while one of them has never imported anything — §17.8's *sources
	// healthy, zero items* is a CLAIM, and this chain is what stops it being made
	// when no import has run.
	//
	// WHAT IT COSTS: on that row, the item-count mark. Nowhere else — a non-zero
	// count reaches this arm with nothing to say anyway — and the count itself is
	// never at stake, since the Items column carries it in every case.
	//
	// ⚠️ IF THIS ORDER LOOKS WRONG, MOVE IT DELIBERATELY AND WITH A TEST, rather
	// than reordering the chain to see what still passes. The zero-item half is
	// pinned as an exact array by *tells NEVER SYNCED apart from connected and
	// empty* (libraries.test.ts); a reorder reintroduces the contradiction there,
	// and loosening that assertion to make it green removes the only guard.
	if (neverSynced.length > 0) {
		// `null` + `0` is http-api.md §3.2's *never synced*, and it is the
		// sentence §17.8 insists is DIFFERENT from "connected and reports 0
		// films". A library whose source has never finished an import is empty
		// because nothing has run, not because the upstream is empty.
		marks.push({
			key: 'never-synced',
			word: plural(neverSynced.length, 'A source has never synced', 'sources have never synced'),
			tone: 'none',
			...detailOf(neverSynced.length === 1 ? neverSynced[0].name : undefined)
		});
	} else if (partial.length > 0) {
		// `null` + `>0`. NOT §17.8's *importing*: nothing on either wire reports an
		// import in flight, and this pair is equally the aftermath of one that
		// failed. The word says what was measured, which is that no import has
		// completed here and rows landed anyway.
		marks.push({
			key: 'partial-import',
			// `An import`, not `A source's import`: measured in Chromium at 1280px,
			// the longer form wrapped the State cell to a second line and took the
			// row from 47px to 63px, for a word the chip beside it already attaches
			// to a source.
			word: plural(partial.length, 'An import did not finish', 'imports did not finish'),
			tone: 'none',
			// Measured at 1280px: the fuller form `some items landed, so this count
			// may be short` rendered 211.66px wide in a 211.66px content box, which
			// is a wrap on the very next character of any longer name or any
			// narrower window. This one is 23 characters and has the margin.
			detail: 'this count may be short'
		});
	} else if (library.itemCount === 0) {
		if (connected.length === verdicts.length) {
			// §17.8's *sources healthy, zero items*, and the join is what makes it
			// sayable: *"Radarr is connected and reports 0 films"* needs `connected`,
			// which is exactly what `GET /api/v1/libraries` never measured. Every
			// source is measured healthy AND every one has completed an import, so
			// the emptiness is the upstream's answer rather than UsArr's silence.
			marks.push({
				key: 'connected-empty',
				word: 'Connected and empty',
				tone: 'none',
				// ⚠️ THE DETAIL STATES THE EVIDENCE, NOT A SECOND COUNT, and the
				// distinction is the one REVIEW-LOG LS-115 insists on. `item_count`
				// is `library_member` rows PER LIBRARY; `work_count` is distinct
				// works PER SERVICE INSTANCE, and an instance can contribute plenty
				// of works while contributing nothing to THIS library. An earlier
				// draft read `Kavita Manga reports no items`, which reads as the
				// second number and would be false in exactly that case. What the
				// join actually measured is that the source is connected and its
				// last import completed, so that is what the line says; the count
				// itself is in the Items column, where it belongs.
				detail: verdicts.length === 1 ? 'its last import finished' : 'their last imports finished'
			});
		} else {
			// The pre-join answer, and it survives for exactly the case it was
			// written for: the count is zero and nothing has measured why.
			marks.push({ key: 'no-items', word: 'No items', tone: 'none' });
		}
	}

	return withVisibility(library, skipMarks(library, completenessMarks(library, marks)));
}

/**
 * WHAT THE IMPORT READ AND DID NOT MAP, AS A ROW MARK.
 *
 * ⚠️ ONE ARM OF THREE READINGS, AND THAT KEEPS THIS FUNCTION'S STANDING
 * INVARIANT INTACT. `none` emits NOTHING and an absent verdict emits nothing, so
 * no positive claim is ever painted on this column — the same treatment
 * `complete` gets in `completenessMarks`, for the same reason. What makes the
 * two silences honest is that they are DIFFERENT VALUES on the wire and the
 * sentence above the table names the second one: a row that says nothing has
 * either nothing to report or nothing to report it with, and `skippedNote` says
 * which.
 *
 * ⚠️ GREY, NOT AMBER, AND IT IS THE OPPOSITE CALL FROM THE SHORTFALL NEXT DOOR.
 * A content filter hiding books is a misconfiguration the owner can go and fix,
 * so it is amber. A skip is UsArr doing what it was built to do — it has no unit
 * of work for a comic — so nothing is broken and there is nothing to go and
 * change. Painting it amber would make the shortfalls harder to find, which is
 * `app.css`'s own argument for `.st--none`.
 *
 * ⚠️ AND IT IS NOT PHRASED AS A COMPLETENESS STATEMENT. The word is about the
 * items that were left out and nothing else; whether the rest of the library
 * arrived is the neighbouring measurement, and the note above the table draws
 * that boundary in words.
 */
function skipMarks(library: Library, marks: LibraryStateMark[]): LibraryStateMark[] {
	const s = library.skipped;
	if (s === undefined || s.state !== 'left_out') return marks;

	const items = s.items ?? 0;
	const mark: LibraryStateMark = {
		key: 'items-left-out',
		// The word is the state; the detail is the evidence and the why. §9.1's
		// split, and the same shape every other mark in this function uses.
		word: 'Some items were left out',
		tone: 'none',
		detail:
			`${COUNT.format(items)} ${items === 1 ? 'item was' : 'items were'} read and not mapped` +
			(s.reason !== undefined ? `; ${s.reason}` : '')
	};
	if (s.recordedAt !== undefined) mark.at = s.recordedAt;
	marks.push(mark);
	return marks;
}

/**
 * THE CONTENT-FILTER SHORTFALL, AS A ROW MARK.
 *
 * ⚠️ TWO ARMS AND NOT THREE. `complete` emits NOTHING, which keeps this
 * function's standing invariant intact: nothing on this screen renders a
 * positive claim. That is not a shortcut here — `complete` is a real,
 * hard-earned measurement — it is that a green tick would fire on nearly every
 * row, which is §17.4 rule 5, and that a positive mark in this column has meant
 * "this library was measured and passed" nowhere else in the product.
 *
 * The two absences still read differently on the screen, which is the constraint
 * the whole feature turns on: a library that was measured and is fine says
 * nothing, and a library that COULD NOT BE MEASURED says exactly that in the
 * `unverified` arm. Silence is only ever "nothing to report", never "checked and
 * fine".
 *
 * ⚠️ AND NEITHER ARM IS PHRASED AS A STATEMENT ABOUT THE SERVICE. The check
 * compares two counts inside one container UsArr can already see; whether whole
 * libraries are hidden from the account is a different question and is not
 * answerable read-only. Every word below is scoped to `this library`.
 */
function completenessMarks(library: Library, marks: LibraryStateMark[]): LibraryStateMark[] {
	const c = library.completeness;
	if (c === undefined) return marks;

	if (c.state === 'shortfall') {
		const total = c.total ?? 0;
		const visible = c.visible ?? 0;
		const mark: LibraryStateMark = {
			key: 'books-hidden',
			// The word is the state; the detail is the evidence. §9.1's split, and
			// the same shape every other mark in this function uses.
			word: 'Some books are hidden',
			tone: 'warn',
			detail: `this library holds ${COUNT.format(total)} ${total === 1 ? 'book' : 'books'}; the service account can see ${COUNT.format(visible)}`
		};
		if (c.checkedAt !== undefined) mark.at = c.checkedAt;
		marks.push(mark);
		return marks;
	}
	if (c.state === 'unverified') {
		const mark: LibraryStateMark = {
			key: 'completeness-unverified',
			// GREY, NOT AMBER. Nothing is broken and nothing is known to be
			// missing; a measurement was not available. Painting a missing
			// measurement the same colour as a real shortfall would make the
			// shortfalls harder to find, which is app.css's own argument for
			// `.st--none`.
			word: 'Completeness unverified',
			tone: 'none',
			detail: c.reason ?? 'UsArr could not check whether every book arrived'
		};
		if (c.checkedAt !== undefined) mark.at = c.checkedAt;
		marks.push(mark);
	}
	return marks;
}

/** The instance kind health reported, for a sentence that names the application. */
function kindOf(source: LibrarySource, read: HealthRead): string {
	if (read.k === 'unread') return source.serviceKind;
	return read.byInstance.get(source.serviceInstanceId)?.kind ?? source.serviceKind;
}

/** A detail slot that stays OFF the object when there is nothing to put in it. */
function detailOf(name: string | undefined): { detail?: string } {
	return name === undefined ? {} : { detail: name };
}

/**
 * §17.8's detail view groups both of these under *Visibility*. They are read
 * from their own columns and are independent of each other, so both can render
 * on one row. Grey is the right role: neither is broken, and painting a
 * deliberate setting a warning colour makes the states that ARE broken harder to
 * find (app.css, `.st--none`).
 */
function withVisibility(library: Library, marks: LibraryStateMark[]): LibraryStateMark[] {
	if (!library.enabled) marks.push({ key: 'disabled', word: 'Turned off', tone: 'none' });
	if (!library.includeInSearch) {
		marks.push({ key: 'not-in-search', word: 'Not in search', tone: 'none' });
	}
	return marks;
}

/* ── 7. the three ways this screen has nothing to show ────────────────────── */

/**
 * A FAILED READ, AN ENDED SESSION AND AN EMPTY INSTALL ARE THREE DIFFERENT
 * THINGS, AND THIS TYPE IS WHY THEY READ DIFFERENTLY.
 *
 * The empty install is not in this union on purpose: it is not a failure, it is
 * a successful read of nothing, and it is the list's own `empty` state
 * (DESIGN-DIRECTION §10). What is here is the two ways the read did not happen.
 *
 * §2.7 gives the endpoint exactly two error statuses — `401 unauthorized` and
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
