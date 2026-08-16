/**
 * IDENTITY-KEYED PER-ROW STATE — ADR-0038 clause 4.
 *
 * ⚠️ SPECIFIED, NOT INCIDENTAL. The authority is `docs/ARCHITECTURE.md` §17.5
 * and `docs/design/DESIGN-DIRECTION.md` §9.1a, settled 2026-08-16 and closed:
 *
 *   "Identity, not position, for focus, hover, selection and pending row state.
 *    Key each to the row's stable id, never to its index — otherwise a reorder
 *    hands the highlight, the focus ring and the in-flight grab to different
 *    rows than the ones they belong to."
 *
 * NOTHING HERE IS SPECIFIC TO THE SEARCH SCREEN, and it is a module rather than
 * page state because the screen that first uses it is scheduled to be replaced.
 *
 * WHAT THIS FILE IS AND IS NOT RESPONSIBLE FOR. Three of clause 4's four states
 * are already identity-keyed by the primitives, and re-implementing them here
 * would be a second mechanism to keep in step:
 *
 *   focus      `$lib/List.svelte` holds `activeKey` and compares it against the
 *              row's `data-key`; `$lib/roving.ts` moves the single tab stop by
 *              the same key. Focus travels with its row already.
 *   hover      CSS `:hover` on the row element. The browser attaches it to the
 *              node, and the keyed `{#each}` moves nodes rather than rewriting
 *              them, so it travels too.
 *   selection  keyed here, because nothing else owns it.
 *   pending    keyed here. THIS IS THE ONE THAT BITES: an in-flight grab held
 *              in an array parallel to the rows hands its spinner — and then
 *              its result — to whichever row later occupies that index.
 *
 * So this module owns the two that have no other home, and says plainly where
 * the other two live rather than duplicating them.
 */

/**
 * A map from row identity to some per-row value, plus a selection set over the
 * same identities.
 *
 * `Id` is the row's stable id: `candidate_id` on a release list. It is never an
 * index and there is deliberately no API here that takes one.
 */
export function createRowState<Id extends string | number, V>() {
	// A plain object rather than a Map: Svelte 5's deep reactivity covers object
	// property assignment, and every read here is a single-key lookup during
	// render, which is where the cost would be.
	let values = $state<Record<string, V>>({});
	// A key map rather than a Set, for the same reason as `values`: it is
	// replaced wholesale on every change, so a reactive Set would be a proxy
	// bought for nothing.
	let selected = $state<Record<string, true>>({});

	const k = (id: Id) => String(id);

	return {
		/** This row's value, or undefined. Never throws on an unknown id: a row
		 * that has just arrived legitimately has no state yet. */
		get(id: Id): V | undefined {
			return values[k(id)];
		},
		set(id: Id, value: V): void {
			values = { ...values, [k(id)]: value };
		},
		delete(id: Id): void {
			const next = { ...values };
			delete next[k(id)];
			values = next;
		},
		/** Every id currently carrying a value. Order is not meaningful. */
		get keys(): string[] {
			return Object.keys(values);
		},

		isSelected(id: Id): boolean {
			return selected[k(id)] === true;
		},
		toggle(id: Id): void {
			const next = { ...selected };
			const key = k(id);
			if (next[key] === true) delete next[key];
			else next[key] = true;
			selected = next;
		},
		get selectedCount(): number {
			return Object.keys(selected).length;
		},

		/**
		 * Drop everything. For a NEW QUERY, not for a new page of one — a "Load
		 * more" append must leave in-flight state alone, which is the same
		 * reason `$lib/List.svelte` re-runs its roving assignment rather than
		 * resetting it.
		 */
		clear(): void {
			values = {};
			selected = {};
		}
	};
}

export type RowState<Id extends string | number, V> = ReturnType<typeof createRowState<Id, V>>;
