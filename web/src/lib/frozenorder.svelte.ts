/**
 * ADR-0038 — "A LIST FREEZES ITS ORDER WHILE A USER IS AIMING AT IT."
 *
 * ⚠️ THE BEHAVIOUR IN THIS FILE IS SPECIFIED, NOT INCIDENTAL, AND IT IS SETTLED.
 * The authority is `docs/ARCHITECTURE.md` §17.5 and
 * `docs/design/DESIGN-DIRECTION.md` §9.1a — settled 2026-08-16 in four rounds
 * across three threads and closed. Anything here that looks like an
 * over-complication is a clause of that rule; check it against the ADR before
 * simplifying it away.
 *
 * NOTHING IN THIS MODULE IS SPECIFIC TO THE SEARCH SCREEN. It attaches to any
 * results list fed by an incremental source, and it is deliberately a module
 * rather than page state because the screen that first uses it is scheduled to
 * be replaced.
 *
 * THE GOVERNING SENTENCE IS THE FIRST CLAUSE, AND IT IS EASY TO LOSE:
 * instability is acceptable only while nobody is aiming at anything. The
 * condition keys on whether a PERSON IS COMMITTED TO A TARGET, not on whether
 * the APPLICATION CONSIDERS ITSELF SETTLED. A re-derivation that keys on
 * fan-out completion alone has already lost the rule.
 *
 * The six clauses, and where each one is:
 *
 *   1. Aim, not readiness.                        — `aimed`, and `region`
 *   2. Re-sort live while the source is still
 *      delivering; freeze on completion until an
 *      explicit re-sort.                          — the `$effect`, and `settle`
 *   3. Freeze while pointer-within OR focus-within.
 *      Anything that would reorder surfaces as ONE
 *      explicit control carrying its own count.    — `region`, `pendingLabel`
 *   4. Identity, not position, for focus, hover,
 *      selection and pending row state.            — `key`, and see below
 *   5. 0 ms, and never animate a re-sort.          — see below
 *   6. Sort key and direction in the URL.          — `$lib/sortspec`
 *
 * ⚠️ TWO DETAILS THAT GET LOST IN A MOVE, WRITTEN DOWN BECAUSE THEY DID:
 *
 *   · THE FREEZE KEYS ON FOCUS AS WELL AS POINTER, and neither half may be
 *     dropped as redundant with the other. Identity-keyed focus (clause 4)
 *     fully protects the keyboard user, because focus travels with its row.
 *     THE PHYSICAL POINTER TRAVELS WITH NOTHING — it sits at a screen
 *     coordinate that no amount of identity keying moves — so a row shifting
 *     under a resting pointer puts a DIFFERENT release under the click, grabbed
 *     irreversibly, with no error raised and nothing on screen recording the
 *     substitution. The two input paths fail differently and both need the
 *     guarantee.
 *
 *   · A LATE STRAGGLER IS NOT A SPECIAL CASE. It is just another thing that
 *     would have reordered, it is counted by the same control, and it DOES NOT
 *     ENTER THE RENDERED LIST until the user re-sorts. There is no
 *     append-below-marked-late mechanism, and that is a rejection rather than
 *     an omission: it is a second rendering rule and a second vocabulary for
 *     one condition, the "late" marker is meaningless the instant the list is
 *     re-sorted, and appending still moves the list under an engaged user —
 *     the row count moves, the scroll extent moves, and a pointer resting near
 *     the foot of the list gets a new row under it.
 *
 * WHY THE RULE IS WORTH A MODULE. The affordance being protected on the first
 * screen to use this is Grab, and a grab is irreversible from UsArr's side: the
 * release goes to a download client UsArr deliberately stops observing after
 * handoff, so a mis-click cannot be detected, reported or reversed. Where there
 * is no undo, prevention is the only lever. A list of purely local reads does
 * not need this and should not pay for it.
 *
 * ON CLAUSE 5, because there is nothing to write here and that is the point:
 * a re-sort is a plain reassignment, there is no transition and there must
 * never be one. An animated reorder is strictly worse than an instant one for
 * this rule — it stretches the window in which the row under the pointer is
 * neither where it was nor where it is going.
 *
 * ON CLAUSE 4, likewise: `key` is required, every comparison in here is by
 * identity, and `$lib/List.svelte` already keys focus and its roving tab stop
 * to the same `data-key` while row hover is CSS on the row element — so both
 * travel with the row for free. What does NOT travel for free is per-row
 * pending state, which is why `$lib/rowstate.svelte` exists beside this.
 */

/** What a caller has to tell the engine. All of it is read reactively. */
export interface FrozenOrderOptions<T> {
	/**
	 * THE ARRIVAL BUFFER: every row known so far, in whatever order they
	 * arrived. This is NOT what gets rendered. Keeping the two apart is the
	 * whole mechanism — the buffer moves freely, the rendered order does not.
	 */
	source: () => readonly T[];
	/** The row's stable identity. Clause 4. Never an index. */
	key: (row: T) => string;
	/** The comparator for the current sort. Re-read on every re-sort, so
	 * changing the sort spec is enough; nothing has to be told twice. */
	compare: () => (a: T, b: T) => number;
	/**
	 * Whether the incremental source has finished delivering. Clause 2: on
	 * completion the order freezes and stays frozen until an explicit re-sort,
	 * so a leg landing after that never silently rearranges a list the user has
	 * begun reading.
	 */
	complete: () => boolean;
}

export interface FrozenOrder<T> {
	/** The rows to render, in the frozen order. A fresh array on every re-sort,
	 * so a consumer that wants `T[]` gets one without aliasing the engine. */
	readonly rows: T[];
	/** True while a person is aiming at the list: pointer-within or
	 * focus-within. Exposed because a screen may legitimately want to say so. */
	readonly aimed: boolean;
	/** How many rows are waiting to enter the rendered list. */
	readonly pendingAdded: number;
	/** How many rendered rows the source has since withdrawn — a superseded
	 * candidate, typically. Counted, never applied behind the user's back:
	 * removing a row moves everything below it. */
	readonly pendingRemoved: number;
	/** Whether the one explicit control should be showing. Clause 3. */
	readonly hasPending: boolean;
	/** The control's own label, carrying its own count. Clause 3. */
	readonly pendingLabel: string;
	/**
	 * THE EXPLICIT RE-SORT. The only thing that may reorder a frozen list, and
	 * therefore the only thing a control may be wired to. Flushes every pending
	 * arrival and removal and re-sorts with the current comparator.
	 */
	resort(): void;
	/** Empty the rendered order — a new query, not a new page of one. */
	reset(): void;
	/** Attach to the RESULTS REGION: the result rows and their header, and
	 * nothing else. A sort control outside this region is always an explicit
	 * re-sort, which is what makes the toolbar the right place for one. */
	region(node: HTMLElement): { destroy(): void };
}

export function createFrozenOrder<T>(options: FrozenOrderOptions<T>): FrozenOrder<T> {
	let rows = $state<T[]>([]);
	let pointerWithin = $state(false);
	let focusWithin = $state(false);

	/**
	 * Clause 3's condition, and clause 1's governing sentence in one line: a
	 * person is committed to a target. OR, never AND — see the file header for
	 * why neither half is redundant.
	 */
	const aimed = $derived(pointerWithin || focusWithin);

	// A plain key map rather than a Set: these are rebuilt from scratch on every
	// change and never mutated, so the reactive Set would add a proxy for
	// nothing, and lookup has to stay O(1) — a linear scan per row makes the
	// pending count O(n²) on a list that can carry hundreds of releases.
	const keyMap = (list: readonly T[]): Record<string, true> => {
		const out: Record<string, true> = {};
		for (const row of list) out[options.key(row)] = true;
		return out;
	};

	const renderedKeys = $derived(keyMap(rows));
	const sourceKeys = $derived(keyMap(options.source()));

	const pendingAdded = $derived(
		options.source().filter((row) => renderedKeys[options.key(row)] !== true).length
	);
	const pendingRemoved = $derived(
		rows.filter((row) => sourceKeys[options.key(row)] !== true).length
	);

	function flush(): void {
		rows = [...options.source()].sort(options.compare());
	}

	/**
	 * THE LIVE HALF OF CLAUSE 2, and the ONLY place the order changes without
	 * the user asking. It is guarded by both halves of the rule: `aimed` is
	 * clause 3, `complete()` is clause 2's freeze-on-completion.
	 *
	 * The completion edge is handled in `settle()` rather than here, because by
	 * the time `complete()` flips the last batch of rows may have landed in the
	 * same tick and this effect would see only the frozen condition — which
	 * would show "12 new results · re-sort" the instant a search finished, for
	 * rows that were never withheld from anybody.
	 */
	$effect(() => {
		if (aimed || options.complete()) return;
		flush();
	});

	return {
		get rows() {
			return rows;
		},
		get aimed() {
			return aimed;
		},
		get pendingAdded() {
			return pendingAdded;
		},
		get pendingRemoved() {
			return pendingRemoved;
		},
		get hasPending() {
			return pendingAdded > 0 || pendingRemoved > 0;
		},
		get pendingLabel() {
			// One control, one label, carrying its own count. A removal with no
			// arrival is real but rare (a higher-priority indexer superseding a
			// candidate), and it has no count worth reading out, so it gets the
			// honest general form rather than a second control.
			if (pendingAdded === 1) return '1 new result · re-sort';
			if (pendingAdded > 1) return `${pendingAdded} new results · re-sort`;
			return 'Results have changed · re-sort';
		},
		resort: flush,
		reset(): void {
			rows = [];
		},
		region(node: HTMLElement) {
			/**
			 * ⚠️ EVERY AIM FLAG IS WRITTEN ONE MICROTASK LATE, AND THAT IS A FIX
			 * FOUND IN A BROWSER RATHER THAN A STYLE CHOICE.
			 *
			 * These are raw DOM listeners, and one of them fires SYNCHRONOUSLY
			 * OUT OF A DOM MUTATION: when a re-sort or an arriving row rebuilds
			 * the blocks holding the focused element, the browser dispatches
			 * FOCUSOUT from inside Svelte's own flush. Assigning `$state` at that
			 * moment is `state_unsafe_mutation` — Svelte throws, the throw
			 * surfaces as an uncaught page error, and THE ASSIGNMENT IS LOST:
			 * the flag being lost is the freeze, and the affordance it protects
			 * is an irreversible grab.
			 *
			 * THE POINTER EVENTS ARE NOT THE CULPRIT, though they read like it.
			 * Chromium never dispatches a pointer boundary event synchronously
			 * out of a mutation — five mutation shapes × four forced-layout
			 * variants, twenty cells, every one landing in a later task. And
			 * focusout throws only from DEEP ENOUGH BLOCK STRUCTURE: a flat
			 * `{#each}` of divs writes the flag legally where `$lib/List.svelte`'s
			 * real nesting throws, which is why three earlier versions of the
			 * scenario passed against broken code. Do not flatten the repro. That
			 * much is measured in Chromium, not reasoned about: re-run the matrix
			 * with `pnpm probe:pointer` rather than trusting the count here.
			 *
			 * WHAT FOLLOWS IS THE REASONING, AND NO TEST ASSERTS IT. The deferral
			 * stays on all five listeners and on `destroy()` regardless, because
			 * one uniform rule is cheaper to keep true than a per-event one; and a
			 * microtask runs after the flush and before the next task, so the flag
			 * is set well before any click can be dispatched against it — a click
			 * is a separate task. Nothing about the rule changes; only the instant
			 * of the write does.
			 */
			const defer = (write: () => void) => queueMicrotask(write);

			const enter = () => {
				defer(() => {
					pointerWithin = true;
				});
			};
			const leave = () => {
				defer(() => {
					pointerWithin = false;
				});
			};
			const focusin = () => {
				defer(() => {
					focusWithin = true;
				});
			};
			// `relatedTarget` is where focus went. Inside the region it is a move
			// between rows, not a departure; null is focus leaving the document
			// entirely, which is a departure.
			const focusout = (event: FocusEvent) => {
				const next = event.relatedTarget;
				if (next instanceof Node && node.contains(next)) return;
				defer(() => {
					focusWithin = false;
				});
			};

			// pointerenter/leave rather than mouseenter/leave: they cover pen and
			// touch, and a touch contact is a person aiming at a row as literally
			// as a mouse is. pointercancel is the case where the browser takes the
			// pointer away (a gesture becoming a scroll), which is a departure.
			node.addEventListener('pointerenter', enter);
			node.addEventListener('pointerleave', leave);
			node.addEventListener('pointercancel', leave);
			node.addEventListener('focusin', focusin);
			node.addEventListener('focusout', focusout);

			return {
				destroy() {
					node.removeEventListener('pointerenter', enter);
					node.removeEventListener('pointerleave', leave);
					node.removeEventListener('pointercancel', leave);
					node.removeEventListener('focusin', focusin);
					node.removeEventListener('focusout', focusout);
					defer(() => {
						pointerWithin = false;
						focusWithin = false;
					});
				}
			};
		}
	};
}

/**
 * The completion edge of clause 2, called by the caller at the moment the
 * source says it has finished.
 *
 * It is a separate call rather than an effect on `complete()` because the two
 * possible states at that instant need different answers and only the caller
 * knows the ordering of its own events:
 *
 *   nobody aiming  — flush once, then the order is frozen for good. The user
 *                    sees the finished list, not a control offering to show it
 *                    to them.
 *   someone aiming — do nothing. The final rows are pending, the control says
 *                    how many, and they enter only when the user asks.
 */
export function settle<T>(order: FrozenOrder<T>): void {
	if (order.aimed) return;
	order.resort();
}
