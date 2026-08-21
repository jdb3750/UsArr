/**
 * THE §17.7 DEGRADED-BACKEND BANNER, as data.
 *
 * One sentence per instance that has stopped answering: the name the user gave
 * it, UsArr's own word for what is wrong, and how old the rows it contributed
 * are. The catalogue keeps rendering underneath it — §17.7 is explicit that a
 * degraded instance greys nothing out, because every row on those screens is a
 * replica row and is exactly as true as it was before the instance went away.
 * §2.3 rule 2 is the same rule at the other altitude: degraded is not blocked.
 *
 * DOM-free and deterministic, for the reason every other `$lib` vocabulary
 * module gives: `vitest.config.ts` is `environment: 'node'` with no Svelte
 * plugin, so a rule that lives inside an `{#if}` in a template is a rule
 * nothing can test. `DegradedBanner.svelte` renders what this decides.
 *
 * WHY THIS IS A SIBLING ABOVE `<List>` AND NOT `List.svelte`'s `staleNote`.
 * Two independent blockers, either of which would be enough. `staleNote` is
 * typed `string`, not `Snippet`, so the link to Services that §17.7 requires
 * cannot pass through it. And `ListState` is a SINGLE value: `'stale'` and
 * `'empty'` are alternatives, so a degraded instance over an empty catalogue
 * could render one or the other and never both — which is precisely the
 * install this banner is for, the one whose only catalogue source is the
 * instance that is down. `List.svelte` also has consumers well beyond these two
 * screens, so widening its prop would put this decision in front of screens
 * §17.7 does not name.
 *
 * ⚠️ `ServiceHealth.stale` IS NOT DATA AGE AND IS NEVER READ HERE. It means
 * "no probe has been taken yet" — `internal/httpapi/services.go` sets it true
 * on every health row and clears it only where a probe snapshot exists. It says
 * nothing about how old the catalogue rows are, which is the only question this
 * banner asks. `ListState`'s `'stale'` is a THIRD unrelated sense in the same
 * tree ("the data is real but old"). Two senses of one word already ship here;
 * this module deliberately adds no third reading of either, and renames
 * neither, because that is its own change with every `<List>` consumer behind
 * it.
 *
 * ⚠️ THE TIMESTAMP IS PER INSTANCE AND THERE IS DELIBERATELY NO GLOBAL ONE.
 * §17.7 fixes it as "that instance's own last successful sync, never the global
 * delta time", and gives the reason: quoting a global freshness figure for an
 * instance that has been failing for two hours overstates it by exactly the
 * interval the user needs. So the name and the number come off the same object.
 *
 * ⚠️ AND IT IS FORMATTED BY `stampOf` FROM ./services, NOT BY `./requests`'s
 * `formatRelative`. The Services screen's `Synced` cell already renders this
 * exact field through `absoluteOf` + `relativeTo`, and a second formatter for
 * one field is how two screens end up disagreeing about the same instant.
 * `stampOf` is also what satisfies DESIGN-DIRECTION §9.1 — absolute plus
 * relative, with a date past 24 hours — which on the one sentence whose whole
 * job is the number is not a formatting preference: "showing cached data from
 * 11:47" reads identically whether the instance has been down for six minutes
 * or twenty-two hours.
 *
 * ⚠️ THE NO-CHANGE-FEED VARIANT IS OUT OF SCOPE, AND SAYING SO IS THE POINT.
 * §17.7 has a second sentence for an instance with no delta channel at all —
 * "showing cached data from the last full compare at 09:12" — and §16.1 funds
 * only the branch that has a sync time. Two reasons stand behind that. No v0.1
 * source is in that state, so the copy could not be produced by anything this
 * milestone runs. And nothing on the wire could tell the two branches apart
 * anyway: `GET /api/v1/services/health` carries `last_full_sync_at` and no
 * per-channel equivalent, which is the same gap `describeSync()` in ./services
 * already diagnoses as the reason it stays uncalled. That is a wire change in
 * another lane, not a rendering decision here.
 */

import type { ServiceHealth, ServicesHealth } from './api';
import { isIndexer, stampOf, stateLabel, type StateLabel, type Tone } from './services';

/** One instance's sentence. Everything the banner renders, already decided. */
export interface DegradedNotice {
	id: number;
	/**
	 * The name the user gave this instance (§17.3), never the kind — which is
	 * what makes the banner legible on a stack running two of the same thing.
	 */
	name: string;
	/** UsArr's own plain word for the state: `not answering`, `paused`. */
	word: string;
	tone: Tone;
	/** The tone's glyph. Decorative — the word is always on screen beside it. */
	icon: StateLabel['icon'];
	/**
	 * Which `app.css` banner modifier carries the tone, decided here rather than
	 * in the template so the choice is something a test can read. Both modifiers
	 * are non-modal and neither greys anything: `--err` is a red hairline and a
	 * red glyph, `--warn` an amber glyph. The LOUD treatment §17.7 asks for is
	 * reserved for its next bullet, the blocking re-identification banner, which
	 * this component is deliberately not.
	 */
	variant: 'banner--err' | 'banner--warn';
	/** `14:02, 6 minutes ago`. Never empty: a notice with no time is not built. */
	stamp: string;
}

/**
 * WHICH SERVER STATES THIS BANNER SPEAKS FOR, and which it deliberately leaves
 * alone.
 *
 * `down` and `degraded` are §17.7's "instance degraded / backend offline"
 * exactly. `needs re-identification` is the NEXT bullet in §17.7 and asks for
 * something this component is not: a BLOCKING banner, on that instance's rows
 * and on the Services screen, carrying a Re-link action. Answering it with a
 * non-modal freshness note would be quieter than the requirement, which on the
 * one state that can destroy a library is the wrong direction to be wrong in.
 * `unknown` is the disabled instance, which is not broken, and `healthy` needs
 * no sentence.
 */
const DEGRADED_STATES = new Set(['down', 'degraded']);

/**
 * Every instance that owes the user a sentence on a catalogue screen, in the
 * order the health response listed them.
 *
 * ONE NOTICE PER INSTANCE, never one merged banner over several. The number is
 * per instance by §17.7's rule above, so a single banner covering two failing
 * instances would have to pick one instance's timestamp and quote it for both —
 * which is the precise overstatement that rule exists to stop, rebuilt at a
 * smaller scale.
 */
export function degradedNotices(health: ServicesHealth, now: Date): DegradedNotice[] {
	return health.services.filter(isDegradedCatalogue).flatMap((h) => {
		const stamp = stampOf(h.lastFullSyncAt ?? undefined, now);
		// NO TIME MEANS NO BANNER, and this is the deliberate `null` decision.
		//
		// `lastFullSyncAt` is `null` for "no full import has ever completed here",
		// which is not a time and must never be softened into one. `syncCell()` in
		// ./services shows why the field cannot be read alone: `null` with a
		// POSITIVE `workCount` is a real and separate state — a partial import
		// leaves its committed batches behind — so there genuinely are replica rows
		// from this instance underneath the table, and there is genuinely no honest
		// instant to date them to.
		//
		// The funded branch (§16.1) is the instance that HAS a sync time, and the
		// whole job of this banner is that number: §17.7 calls it "the whole job of
		// the banner", and a version with the number removed is a different, quieter
		// component that was not asked for. So both null states — never imported,
		// and partially imported — are silent HERE, and neither goes unreported:
		// the instance is on the Services screen with `Never` or `Partial import`
		// in its own `Synced` cell, and Home's Block B is the attention block that
		// names a degraded instance (§17.2).
		//
		// `stampOf` is the parse as well as the format, so an unparseable timestamp
		// falls into this branch rather than rendering a banner with a blank where
		// the number goes.
		if (stamp === '') return [];
		// The Services screen's own State column, reused rather than re-derived, so
		// the word in the banner and the word on the row that fixes it cannot
		// diverge. §17.3 owns that vocabulary: `not answering`, never `down`.
		const state = stateLabel({ health: h }, now);
		return [
			{
				id: h.id,
				name: h.name,
				word: state.word,
				tone: state.tone,
				icon: state.icon,
				variant: state.tone === 'err' ? 'banner--err' : 'banner--warn',
				stamp
			}
		];
	});
}

/**
 * ⚠️ AN INDEXER IS EXCLUDED, AND NOT BECAUSE IT MATTERS LESS. The banner's
 * sentence is a claim about CACHED CATALOGUE DATA, and an indexer contributes
 * none — which is why `syncCell()` answers `Not applicable / no catalogue` for
 * one rather than `Never`. "Showing cached data from 14:02" over a broken
 * Prowlarr would date rows it did not write. A broken indexer breaks SEARCH AND
 * GRAB, and the screens that own that state say so there.
 */
function isDegradedCatalogue(h: ServiceHealth): boolean {
	// A disabled instance cannot reach `down` or `degraded` on today's server —
	// it is mapped to `unknown` before either test runs — but this reads the wire
	// rather than that mapping: `stateLabel` answers `turned off` for it, which
	// is not a sentence about a broken backend.
	if (!h.enabled) return false;
	if (isIndexer(h)) return false;
	return DEGRADED_STATES.has(h.state);
}
