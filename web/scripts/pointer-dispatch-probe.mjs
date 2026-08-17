/**
 * =============================================================================
 * THE PROBE BEHIND THE MATRIX QUOTED IN freeze-check.mjs.
 * =============================================================================
 *
 * freeze-check.mjs asserts, as measurement rather than reasoning, that Chromium
 * NEVER dispatches a pointer boundary event synchronously out of a DOM
 * mutation. That claim is the load-bearing half of ADR-0038's explanation: it
 * is the reason `pointerenter` is NOT the event that can be swallowed by
 * `state_unsafe_mutation`, and therefore the reason the file header of
 * frozenorder.svelte.ts names `focusout` instead.
 *
 * A claim like that is worth exactly as much as the instrument that produced
 * it, so the instrument lives here rather than in somebody's memory. Run it and
 * the matrix reprints.
 *
 * WHAT IT MEASURES. Five mutation shapes crossed with four forced-layout
 * variants. For each cell: park a REAL pointer over a row, mutate, and record
 * which phase every boundary event lands in —
 *
 *   mutation  during the mutating statement, before it returns   ← the finding
 *   read      during the forced-layout read that follows it
 *   task      the remainder of that same task
 *   later     any subsequent task, microtask or frame
 *
 * WHY PLAIN DOM AND NOT THE SVELTE HARNESS. The claim is about Blink's dispatch
 * timing, which is decided below any framework. Plain DOM is the minimal
 * instrument that can hold it, and it keeps the probe from inheriting Svelte's
 * scheduler as a confound. freeze-check.mjs is where the rule meets the real
 * primitive; this is where the browser fact is established.
 *
 * ⚠️ THE POSITIVE CONTROL IS THE POINT. An instrument that cannot see a
 * synchronous dispatch reports "nothing was synchronous" against every input,
 * including a broken one — which is indistinguishable from the finding. So the
 * run ends by removing a FOCUSED element and requiring `focusout` to land in
 * the `mutation` phase. If that control does not fire, the matrix above it is
 * void and the probe exits non-zero without reporting a result.
 *
 * The second control is per cell: every cell also reports how many boundary
 * events arrived LATER. A cell that saw no events at all in any phase proves
 * nothing about ordering — it just failed to provoke the browser — so cells are
 * only counted as evidence once the mutation is shown to have moved the hover
 * chain at all.
 *
 * Run: PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers pnpm probe:pointer
 * =============================================================================
 */
import { createRequire } from 'node:module';
import { chromium } from 'playwright-core';

const pwVersion = createRequire(import.meta.url)('playwright-core/package.json').version;

if (!process.env.PLAYWRIGHT_BROWSERS_PATH)
	process.env.PLAYWRIGHT_BROWSERS_PATH = '/opt/pw-browsers';

const ROWS = 20;
const HOVERED = 10;

/* The five shapes named in freeze-check.mjs, in the order it names them. */
const SHAPES = [
	['remove', 'remove the hovered row'],
	['move', 'move the hovered row'],
	['prepend', 'prepend a row above it'],
	['reorder', 'reorder all twenty'],
	['collapse', 'collapse the region']
];

/* Three forced-layout reads, plus the no-read control. */
const VARIANTS = ['none', 'offsetHeight', 'getBoundingClientRect', 'elementFromPoint'];

const PAGE = `<!doctype html>
<meta charset="utf-8">
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font: 14px system-ui; }
  #region { width: 400px; }
  .row { height: 24px; line-height: 24px; padding-left: 8px; border-bottom: 1px solid #ddd; }
  .cell { display: inline-block; width: 100px; }
</style>
<div id="region">${Array.from(
	{ length: ROWS },
	(_, i) => `<div class="row" id="r${i}" tabindex="0"><span class="cell">row ${i}</span></div>`
).join('')}</div>
<script>
  // Boundary events only. pointerover/out bubble; pointerenter/leave do not,
  // so those are bound per row and rebound for any row created mid-run.
  window.__log = [];
  window.__phase = 'idle';
  const BUBBLING = ['pointerover', 'pointerout', 'mouseover', 'mouseout'];
  const DIRECT = ['pointerenter', 'pointerleave', 'mouseenter', 'mouseleave'];
  const record = (e) => {
    window.__log.push({
      type: e.type,
      id: e.currentTarget && e.currentTarget.id ? e.currentTarget.id : '(doc)',
      phase: window.__phase,
    });
  };
  for (const t of BUBBLING) document.addEventListener(t, record, true);
  window.__bind = (el) => { for (const t of DIRECT) el.addEventListener(t, record); };
  window.__bind(document.getElementById('region'));
  for (const el of document.querySelectorAll('.row')) window.__bind(el);
  // focus events, for the positive control only.
  window.__bindFocus = () => {
    for (const t of ['focusout', 'focusin', 'blur', 'focus'])
      document.addEventListener(t, record, true);
  };
</script>`;

/* --- reporting ------------------------------------------------------------ */

const bold = (s) => `[1m${s}[0m`;
const red = (s) => `[31m${s}[0m`;
const green = (s) => `[32m${s}[0m`;

/* --- the run -------------------------------------------------------------- */

const browser = await chromium.launch();

/**
 * One cell of the matrix. Returns the phase tally for the boundary events the
 * mutation provoked.
 *
 * ⚠️ A FRESH PAGE PER CELL, AND NOT FOR TIDINESS. Reusing one page across cells
 * and re-running `setContent` silently disarms the probe: by the third cell the
 * setup `mouse.move` stops dispatching anything, and Blink then updates the
 * `:hover` chain from its cached pointer position WITHOUT dispatching boundary
 * events. Every cell after that reports "no events" — which reads exactly like
 * the finding while actually measuring nothing. Measured here before it was
 * trusted: reused page, 14 setup events on cell 1, 6 on cell 2, 0 thereafter;
 * fresh page, 14 every time.
 */
async function cell(shape, variant) {
	const page = await browser.newPage({ viewport: { width: 800, height: 700 } });
	try {
		return await run(page, shape, variant);
	} finally {
		await page.close();
	}
}

async function run(page, shape, variant) {
	await page.setContent(PAGE);

	// Park a real pointer over the hovered row. Two moves: setContent does not
	// re-dispatch a move, so the first lands the cursor somewhere neutral and
	// the second establishes the hover chain on the new content.
	const box = await page.locator(`#r${HOVERED}`).boundingBox();
	const px = Math.round(box.x + box.width / 2);
	const py = Math.round(box.y + box.height / 2);
	await page.mouse.move(5, 5);
	await page.mouse.move(px, py);
	await page.waitForFunction((id) => document.getElementById(id)?.matches(':hover'), `r${HOVERED}`);

	const setupEvents = await page.evaluate(() => window.__log.length);

	await page.evaluate(
		({ shape, variant, px, py, hovered }) => {
			const region = document.getElementById('region');
			const row = document.getElementById('r' + hovered);
			window.__log.length = 0;

			const mutate = () => {
				if (shape === 'remove') row.remove();
				else if (shape === 'move') region.appendChild(row);
				else if (shape === 'prepend') {
					const fresh = document.createElement('div');
					fresh.className = 'row';
					fresh.id = 'rNew';
					fresh.tabIndex = 0;
					fresh.innerHTML = '<span class="cell">row new</span>';
					window.__bind(fresh);
					region.insertBefore(fresh, region.firstElementChild);
				} else if (shape === 'reorder') {
					for (const r of [...region.children].reverse()) region.appendChild(r);
				} else if (shape === 'collapse') region.style.display = 'none';
			};

			const read = () => {
				if (variant === 'offsetHeight') return void document.body.offsetHeight;
				if (variant === 'getBoundingClientRect') return void document.body.getBoundingClientRect();
				if (variant === 'elementFromPoint') return void document.elementFromPoint(px, py);
			};

			window.__phase = 'mutation';
			mutate();
			window.__phase = 'read';
			read();
			window.__phase = 'task';
			queueMicrotask(() => {
				window.__phase = 'later';
			});
			setTimeout(() => {
				window.__phase = 'later';
			}, 0);
			requestAnimationFrame(() => {
				window.__phase = 'later';
			});
		},
		{ shape, variant, px, py, hovered: HOVERED }
	);

	// Give Blink its post-lifecycle hover recompute: several frames, then a beat.
	await page.evaluate(
		() =>
			new Promise((r) => {
				let n = 0;
				const tick = () => (++n < 5 ? requestAnimationFrame(tick) : setTimeout(r, 120));
				requestAnimationFrame(tick);
			})
	);

	const all = await page.evaluate(() => window.__log.slice());
	const tally = { mutation: 0, read: 0, task: 0, later: 0 };
	for (const e of all) tally[e.phase] = (tally[e.phase] ?? 0) + 1;
	return { tally, setupEvents, types: [...new Set(all.map((e) => e.type))] };
}

console.log(bold('\npointer boundary dispatch vs DOM mutation — Chromium'));
console.log(`  chromium ${await browser.version()}   playwright-core ${pwVersion}`);
console.log(`  ${ROWS} rows, pointer parked on row ${HOVERED}\n`);

const width = Math.max(...SHAPES.map(([, label]) => label.length));
console.log(
	bold(`  ${'mutation shape'.padEnd(width)}  ` + VARIANTS.map((x) => x.padEnd(22)).join(''))
);

let syncFindings = 0;
let inertCells = 0;
let unarmedCells = 0;
let cells = 0;
const seenTypes = new Set();

for (const [shape, label] of SHAPES) {
	const parts = [];
	for (const variant of VARIANTS) {
		const { tally, setupEvents, types } = await cell(shape, variant);
		cells++;
		// Arming check: the setup mouse move must itself have dispatched. If it
		// did not, this cell never had a live hover chain to disturb.
		if (setupEvents === 0) unarmedCells++;
		for (const t of types) seenTypes.add(t);
		const isSync = tally.mutation > 0 || tally.read > 0 || tally.task > 0;
		if (isSync) syncFindings++;
		const moved = tally.mutation + tally.read + tally.task + tally.later;
		if (moved === 0) inertCells++;
		// Build the plain text first and colour it after, so the column padding is
		// computed on the visible width rather than on the escape sequences.
		const plain = isSync
			? `SYNC(${tally.mutation}/${tally.read}/${tally.task}) later:${tally.later}`
			: moved === 0
				? '— no events'
				: `async only, later:${tally.later}`;
		parts.push((isSync ? red(plain) : plain) + ' '.repeat(Math.max(1, 22 - plain.length)));
	}
	console.log(`  ${label.padEnd(width)}  ` + parts.join(''));
}

/* --- the positive control ------------------------------------------------- */

console.log(bold('\n  positive control — can this probe see a synchronous dispatch at all?'));

const controlPage = await browser.newPage({ viewport: { width: 800, height: 700 } });
await controlPage.setContent(PAGE);
const control = await controlPage.evaluate((hovered) => {
	window.__bindFocus();
	const row = document.getElementById('r' + hovered);
	row.focus();
	window.__log.length = 0;
	window.__phase = 'mutation';
	row.remove();
	window.__phase = 'task';
	return {
		log: window.__log.slice(),
		active: document.activeElement ? document.activeElement.tagName : null
	};
}, HOVERED);

const controlSync = control.log.filter((e) => e.phase === 'mutation' && e.type === 'focusout');
const controlOk = controlSync.length > 0 && control.active === 'BODY';
console.log(
	controlOk
		? `  ${green('ok')}   removing a focused element dispatched focusout in the mutation phase ` +
				`(activeElement → <${control.active.toLowerCase()}>)`
		: `  ${red('FAIL')} no synchronous focusout — the probe cannot see sync dispatch, ` +
				`so the matrix above is void`
);

await controlPage.close();
await browser.close();

/* --- verdict -------------------------------------------------------------- */

console.log('');
console.log(`  boundary event types observed: ${[...seenTypes].sort().join(', ') || '(none)'}`);
if (unarmedCells > 0) {
	console.log(
		red(
			`  VOID: ${unarmedCells}/${cells} cells never dispatched a setup event, ` +
				`so they had no live hover chain to disturb.`
		)
	);
	process.exit(1);
}
if (!controlOk) {
	console.log(red('  VOID: positive control did not fire. Matrix means nothing.'));
	process.exit(1);
}
if (inertCells > 0) {
	console.log(
		red(
			`  VOID: ${inertCells}/${cells} cells provoked no boundary event in any phase, ` +
				`so they are not evidence of ordering.`
		)
	);
	process.exit(1);
}
if (syncFindings > 0) {
	console.log(
		red(
			`  FINDING: ${syncFindings}/${cells} cells dispatched a boundary event ` +
				`synchronously. This CONTRADICTS freeze-check.mjs — stop and read it.`
		)
	);
	process.exit(1);
}
console.log(
	green(
		`  ${cells}/${cells} cells: every boundary event arrived in a later task. ` +
			`Confirms freeze-check.mjs.`
	)
);
