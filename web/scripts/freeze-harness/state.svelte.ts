/**
 * The arrival buffer the harness feeds `createFrozenOrder` from.
 *
 * FABRICATED DATA, DEV ONLY. Never imported by anything under src/routes.
 *
 * This models the ONE thing ADR-0038 exists for: an incremental source that is
 * still delivering. `complete` stays false for every scenario in
 * `scripts/freeze-check.mjs`, because the failure window the negative control
 * has to cover is the LIVE FAN-OUT — legs still landing, rows still arriving,
 * and a Grab under the pointer that cannot be taken back.
 */

export interface FeedRow {
	id: string;
	/** Sorted DESCENDING on this, so a high-ranked arrival lands at the top and
	 * moves every rendered row down by one. That is the motion the rule forbids
	 * while somebody is aiming. */
	rank: number;
	label: string;
}

let rows = $state<FeedRow[]>([]);
let complete = $state(false);
let descending = $state(true);

export const feed = {
	get rows() {
		return rows;
	},
	get complete() {
		return complete;
	},
	setComplete(next: boolean) {
		complete = next;
	},
	/** ADR-0038 clause 6 — the sort direction is an input, and flipping it is the
	 * ordinary way a user asks for an explicit re-sort. It is here because a
	 * flip is the only re-sort that MOVES every rendered row's DOM element, and
	 * moving the element that holds focus is what makes the browser fire
	 * `focusout` synchronously out of the mutation. */
	get descending() {
		return descending;
	},
	flipDirection() {
		descending = !descending;
	},
	/** Empty the buffer. Paired with `order.reset()` by the driver. */
	clear() {
		rows = [];
	},
	/**
	 * Append `count` rows whose ranks sit BELOW everything already buffered, so
	 * they sort to the foot of the list. Used to grow the region downward under
	 * a resting pointer without disturbing the rows already rendered.
	 */
	appendBelow(count: number) {
		const floor = rows.length === 0 ? 1_000_000 : Math.min(...rows.map((r) => r.rank));
		const extra: FeedRow[] = [];
		for (let i = 0; i < count; i++) {
			const rank = floor - 1 - i;
			extra.push({ id: `low-${rank}`, rank, label: `Arrival rank ${rank}` });
		}
		rows = [...rows, ...extra];
	},
	/**
	 * Append ONE row that outranks everything buffered. This is the arrival that
	 * would reorder: it sorts to index 0 and pushes every rendered row down a
	 * position. Whether it does is the whole assertion.
	 */
	appendAbove() {
		const ceiling = rows.length === 0 ? 0 : Math.max(...rows.map((r) => r.rank));
		const rank = ceiling + 1;
		rows = [...rows, { id: `high-${rank}`, rank, label: `Arrival rank ${rank}` }];
	}
};
