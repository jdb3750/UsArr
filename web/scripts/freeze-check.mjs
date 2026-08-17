/**
 * =============================================================================
 * THE NEGATIVE CONTROL FOR ADR-0038 — "a list freezes its order while a user is
 * aiming at it."
 * =============================================================================
 *
 * WHY THIS IS A BROWSER SCRIPT AND NOT A VITEST FILE. `vitest.config.ts` is
 * `environment: 'node'`. The rule under test arms from RAW DOM LISTENERS
 * reacting to events a browser dispatches out of a DOM mutation, at a moment
 * decided by hit testing against a physical pointer coordinate. None of that
 * exists in the unit-test run — no layout, no hit test, no pointer, no focus
 * ring — so the unit run can assert that an intent flag was set and nothing
 * more.
 *
 * ⚠️ AND ASSERTING THAT AN INTENT FLAG WAS SET IS EXACTLY THE CHECK THAT PASSED
 * WHILE THE RULE WAS BROKEN. `region()` shipped writing its aim flags
 * synchronously. Some of those events fire from inside Svelte's own flush,
 * where writing `$state` raises `state_unsafe_mutation` and THE ASSIGNMENT IS
 * DISCARDED rather than deferred. Seven green checks said the freeze had armed.
 * So every assertion below is on OBSERVABLE ORDER read back off the DOM — the
 * keys of the rendered rows, in the order a person could point at them — and
 * never on `aimed`, `pointerWithin`, or any other statement of intent.
 *
 * -----------------------------------------------------------------------------
 * ⚠️ WHICH EVENT CAN ACTUALLY BE LOST, MEASURED IN THIS CHROMIUM RATHER THAN
 * REASONED ABOUT. Three attempts at scenario 5 went green against the pre-fix
 * code before this was measured instead of assumed, so it is written down.
 *
 *   · POINTER BOUNDARY EVENTS ARE NEVER DISPATCHED SYNCHRONOUSLY OUT OF A DOM
 *     MUTATION. Removing the hovered row, moving it, prepending a row above it,
 *     reordering all twenty, collapsing the region out from under a parked
 *     pointer — none of them dispatch anything during the mutation, with or
 *     without a forced layout read (offsetHeight / getBoundingClientRect /
 *     elementFromPoint) in the same task. Every pointerenter and pointerleave
 *     arrives in a task of its own, where writing `$state` is legal. FIVE
 *     mutation shapes — the five just named — crossed with four forced-layout
 *     variants, being those three reads plus a no-read control: twenty cells,
 *     all measured, all async-only. (INFERENCE, not measured: the mechanism is
 *     presumably Blink recomputing the hover chain in the frame's
 *     post-lifecycle step. The probe times the dispatch; it does not attribute
 *     it.)
 *     The probe is `scripts/pointer-dispatch-probe.mjs` — `pnpm probe:pointer`
 *     reprints the matrix, and carries its own positive control, because a
 *     number nobody can re-derive is a number on nobody's authority. Note what
 *     this means for the file header of frozenorder.svelte.ts: the deferral is
 *     right, but pointerenter is not the event that demonstrates it.
 *   · `focusout` IS SYNCHRONOUS. Removing the focused element — or MOVING it,
 *     which is a remove plus an insert, and which is what a keyed `{#each}`
 *     does to every row on a re-sort — dispatches `focusout` from inside the
 *     mutation, before the mutating statement returns. `document.activeElement`
 *     becomes `<body>`, and no `focusin` follows.
 *   · AND IT ONLY THROWS FROM DEEP ENOUGH INSIDE THE BLOCK STRUCTURE. `set()`
 *     raises `state_unsafe_mutation` on the reaction that is active AT THE
 *     INSTANT OF THE WRITE, so it depends on where the moved node sits in what
 *     Svelte is reconciling. A flat `{#each}` of `<div>` rows wrote the flag
 *     legally and the pre-fix code passed; `$lib/List.svelte`, whose rows each
 *     hold their own keyed `{#each}` of cells, throws. That is why the harness
 *     renders the real primitive — see the header of Region.svelte.
 *
 * So the write a browser can really throw away here is a focus write, and it is
 * thrown away in the direction that LATCHES THE FREEZE ON: the flag that fails
 * to clear is `focusWithin`. Scenario 5 is that case, and it is the scenario
 * that goes red against the pre-fix code — that much is measured.
 *
 * WHY THAT DIRECTION IS THE BAD ONE IS REASONING, NOT MEASUREMENT, and is
 * recorded as the rationale for spending a scenario on it: a list whose freeze
 * never releases stops re-sorting for the rest of the session and withholds
 * every later arrival from a user who is not there, behind a control they have
 * no reason to press — the failure mode that looks like nothing being wrong.
 * No test here asserts that severity; it is an argument about consequences.
 *
 * The pointer scenarios stay, and they are not decoration: they assert the
 * ARMING direction on observable order, which is the check the original
 * verification should have been. Scenario 2 is their control run — without it,
 * "the list did not move" cannot be told apart from "the list never moves".
 * -----------------------------------------------------------------------------
 *
 * HOW TO FIRE IT DELIBERATELY, which is the only reason to trust it. Replace
 * the body of `defer` in src/lib/frozenorder.svelte.ts with `write()` — the
 * pre-fix code — and run this again. Scenario 5 must go red, reporting
 * `order.aimed` still true with focus on `<body>` and one uncaught
 * `state_unsafe_mutation`. If it stays green this script has stopped measuring
 * the rule, and its green means nothing.
 *
 * Run: PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers pnpm test:freeze
 * =============================================================================
 */
import { createServer } from 'node:http';
import { rm } from 'node:fs/promises';
import { createReadStream } from 'node:fs';
import { dirname, extname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { tmpdir } from 'node:os';

const here = dirname(fileURLToPath(import.meta.url));
const webDir = resolve(here, '..');
const harnessDir = join(here, 'freeze-harness');
const outDir = join(tmpdir(), `usarr-freeze-check-${process.pid}`);

/* --- reporting ------------------------------------------------------------ */

let failures = 0;
const okMark = '  [32mok[0m ';
const failMark = '  [31mFAIL[0m ';

function head(text) {
	process.stdout.write(`\n[1m${text}[0m\n`);
}
function ok(text) {
	process.stdout.write(okMark + text + '\n');
}
function fail(text) {
	failures++;
	process.stdout.write(failMark + text + '\n');
}
function note(text) {
	process.stdout.write('     ' + text + '\n');
}
function assert(condition, passText, failText) {
	if (condition) ok(passText);
	else fail(failText);
}

/* --- build the harness ---------------------------------------------------- */

const { build } = await import('vite');
const { svelte, vitePreprocess } = await import('@sveltejs/vite-plugin-svelte');

await build({
	root: harnessDir,
	base: './',
	logLevel: 'warn',
	configFile: false,
	// The harness builds the REAL module out of src/lib. It does not carry a
	// copy: a control that exercises a copy controls nothing.
	resolve: {
		alias: {
			$lib: join(webDir, 'src', 'lib'),
			// SvelteKit's module, which does not exist outside a SvelteKit build.
			// Shared with scripts/harness rather than copied.
			'$app/environment': join(here, 'harness', 'app-environment.ts')
		}
	},
	plugins: [svelte({ configFile: false, preprocess: vitePreprocess() })],
	build: { outDir, emptyOutDir: true, target: 'esnext', minify: false }
});

/* --- serve it ------------------------------------------------------------- */

const MIME = {
	'.html': 'text/html; charset=utf-8',
	'.js': 'text/javascript; charset=utf-8',
	'.css': 'text/css; charset=utf-8'
};

const server = createServer((req, res) => {
	const path = (req.url ?? '/').split('?')[0];
	const file = join(outDir, path === '/' ? 'index.html' : decodeURIComponent(path));
	if (!file.startsWith(outDir)) {
		res.writeHead(403).end();
		return;
	}
	const stream = createReadStream(file);
	stream.on('error', () => res.writeHead(404).end());
	stream.on('open', () => {
		res.writeHead(200, { 'content-type': MIME[extname(file)] ?? 'application/octet-stream' });
		stream.pipe(res);
	});
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const URL_BASE = `http://127.0.0.1:${server.address().port}/`;

/* --- drive it ------------------------------------------------------------- */

if (!process.env.PLAYWRIGHT_BROWSERS_PATH)
	process.env.PLAYWRIGHT_BROWSERS_PATH = '/opt/pw-browsers';
const { chromium } = await import('playwright-core');
const browser = await chromium.launch();
const context = await browser.newContext({ viewport: { width: 1000, height: 800 } });

/**
 * A FRESH PAGE PER SCENARIO, and this is load-bearing rather than tidiness.
 * Blink tracks one "element under the mouse" per document and drops it, without
 * dispatching anything, when that element is removed. A scenario that collapsed
 * the list therefore leaves the next scenario's pointer state unrecoverable
 * from script — the region can be left permanently believing a pointer is in
 * it, and the run after would pass or fail on the run before.
 */
async function freshPage() {
	const page = await context.newPage();
	page.on('pageerror', () => {}); // collected in-page; see main.ts
	await page.goto(URL_BASE);
	await page.waitForSelector('[data-testid="region"] table');
	await page.mouse.move(5, 5);
	await tick(page);
	await page.evaluate(() => {
		window.__freeze.pageErrors.length = 0;
		window.__freeze.handle.drainObserved();
	});
	return page;
}

/** One task boundary. The shipped fix writes its aim flags one microtask late,
 * and a microtask boundary only exists between tasks — so a control that did
 * everything in one task would fail the FIXED code and prove nothing about the
 * broken one. Every arming step is separated from the step it arms by this. */
const tick = (page) => page.evaluate(() => new Promise((r) => setTimeout(r, 0)));

/** Two frames. Blink dispatches pointer boundary events in the frame's
 * post-lifecycle step, so a pointer that has been moved needs a frame before
 * the region has heard about it. */
const frames = (page) =>
	page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));

/** Everything the assertions read, in one round trip.
 *
 * ⚠️ The one `textContent` here is on `[data-testid="pending"]`, which is a `<p>`
 * holding one interpolated string and — audited rather than assumed — ZERO child
 * elements, in both the pending and the no-pending state. That matters because
 * `textContent` reads through `display: none` and through `[hidden]` alike, so
 * the same call over a tree carrying a hidden variant of its visible text would
 * return a sentence no user has seen. The region's own table is such a tree
 * (`.stacklabel` is `display: none` at desktop width and lives inside the
 * `<td>`s), which is why the order assertions read `renderedKeys()` off row
 * `data-key` attributes and never off row text. Keep it that way. */
const snapshot = (page) =>
	page.evaluate(() => ({
		rendered: window.__freeze.renderedKeys(),
		buffered: window.__freeze.feed.rows.map((r) => r.id),
		pendingAdded: window.__freeze.handle.order.pendingAdded,
		aimed: window.__freeze.handle.order.aimed,
		pending: document.querySelector('[data-testid="pending"]').textContent.trim(),
		activeKey: document.activeElement?.dataset?.key ?? document.activeElement?.tagName ?? null,
		errors: [...window.__freeze.pageErrors],
		observed: window.__freeze.handle
			.drainObserved()
			.map((o) => `${o.type}${o.duringFlush ? '*' : ''}`)
	}));

/** Park a REAL pointer on the centre of a rendered row and leave it there.
 * Not `dispatchEvent(new PointerEvent(...))`, which would fire the listener at
 * a moment of the test's choosing and so measure the test's timing rather than
 * the browser's. */
async function parkOnRow(page, index) {
	const at = await page.evaluate((i) => {
		const row = document.querySelectorAll(window.__freeze.rowSelector)[i];
		const r = row.getBoundingClientRect();
		return {
			x: Math.round(r.x + r.width / 2),
			y: Math.round(r.y + r.height / 2),
			key: row.dataset.key
		};
	}, index);
	await page.mouse.move(at.x, at.y);
	await frames(page);
	await tick(page);
	return at;
}

/** Fill the arrival buffer in ONE Svelte flush, which is what an SSE leg does. */
const arrive = (page, n) =>
	page.evaluate((count) => window.__freeze.flush(() => window.__freeze.feed.appendBelow(count)), n);

/** THE ARRIVAL THAT WOULD REORDER: one candidate outranking everything on
 * screen. Unfrozen it sorts to index 0 and moves every rendered row down by one
 * row height, including whichever row is under the pointer. The affordance in
 * that row is Grab, and a grab is irreversible. */
const arriveAbove = (page) =>
	page.evaluate(() => window.__freeze.flush(() => window.__freeze.feed.appendAbove()));

/* =============================================================================
 * 1. POINTER. A resting pointer inside the region, and an arrival that would
 *    reorder the list under it.
 * ========================================================================== */

head('1. Pointer resting inside the region: an arrival must not reorder the list');
{
	const page = await freshPage();
	await arrive(page, 20);
	await tick(page);
	const at = await parkOnRow(page, 8);

	const armed = await snapshot(page);
	note(`pointer parked on ${at.key} at (${at.x}, ${at.y})`);
	note(`boundary events (* = dispatched inside a flush): ${armed.observed.join(', ') || 'none'}`);
	assert(
		armed.observed.some((o) => o.startsWith('pointerenter')),
		'the browser dispatched pointerenter for the parked pointer',
		'no pointerenter was dispatched — the pointer never entered the region, so this scenario tested nothing'
	);

	const headBefore = armed.rendered[0];
	await arriveAbove(page);
	const after = await snapshot(page);

	assert(
		after.rendered[0] === headBefore && after.rendered.length === armed.rendered.length,
		`the arrival did not enter: head still ${headBefore}, still ${after.rendered.length} rows`,
		`THE LIST REORDERED UNDER A RESTING POINTER. Head ${headBefore} → ${after.rendered[0]}, ` +
			`${armed.rendered.length} → ${after.rendered.length} rows. The freeze did not arm` +
			(after.errors.length > 0 ? ` — uncaught: ${after.errors.join(' | ')}` : '')
	);
	assert(
		after.pending.includes('re-sort'),
		`the one explicit control is offering it instead: "${after.pending}"`,
		`the pending control says "${after.pending}" — the arrival was neither shown nor offered`
	);
	await page.close();
}

/* =============================================================================
 * 2. THE CONTROL RUN. Without it, "the list did not move" cannot be told apart
 *    from "the list never moves", and scenario 1 has no teeth.
 * ========================================================================== */

head('2. Control: with nobody aiming, the same arrival re-sorts live');
{
	const page = await freshPage();
	await arrive(page, 20);
	await tick(page);
	const before = await snapshot(page);
	await arriveAbove(page);
	const after = await snapshot(page);

	assert(
		after.rendered[0] !== before.rendered[0] &&
			after.rendered.length === before.rendered.length + 1,
		`unaimed, the arrival went straight in at the head (${before.rendered[0]} → ${after.rendered[0]}, ` +
			`${before.rendered.length} → ${after.rendered.length} rows)`,
		`unaimed, the list did NOT re-sort live (head ${before.rendered[0]} → ${after.rendered[0]}). ` +
			`Clause 2's live half is broken and scenario 1 is measuring nothing`
	);
	await page.close();
}

/* =============================================================================
 * 3. FOCUS. The other half of clause 3, with the pointer nowhere near.
 * ========================================================================== */

head('3. Focus inside the region: the same arrival must not reorder the list');
{
	const page = await freshPage();
	await arrive(page, 8);
	await tick(page);
	const focusedKey = await page.evaluate(() => {
		const row = document.querySelectorAll(window.__freeze.rowSelector)[3];
		row.focus();
		return row.dataset.key;
	});
	await tick(page);

	const armed = await snapshot(page);
	note(`focus on ${focusedKey}; boundary events: ${armed.observed.join(', ') || 'none'}`);
	const headBefore = armed.rendered[0];

	await arriveAbove(page);
	const after = await snapshot(page);

	assert(
		after.rendered[0] === headBefore && after.rendered.length === armed.rendered.length,
		`with focus inside, the arrival did not enter: head still ${headBefore}, ${after.rendered.length} rows`,
		`the list reordered with focus inside the region: head ${headBefore} → ${after.rendered[0]}`
	);
	assert(
		after.activeKey === focusedKey,
		`focus stayed on ${focusedKey} across the arrival`,
		`focus moved from ${focusedKey} to ${after.activeKey} across the arrival`
	);
	await page.close();
}

/* =============================================================================
 * 4. RELEASE. Aim is a condition, not a latch: a pointer that leaves must let
 *    the withheld arrivals in, or "frozen" only ever means "stuck".
 * ========================================================================== */

head('4. Release: the pointer leaves, and the withheld arrivals enter');
{
	const page = await freshPage();
	await arrive(page, 20);
	await tick(page);
	await parkOnRow(page, 8);
	await arriveAbove(page);
	const frozen = await snapshot(page);
	assert(
		frozen.pendingAdded === 1,
		'one arrival is withheld while the pointer rests inside',
		`expected 1 withheld arrival, found ${frozen.pendingAdded} — the pointer is not registering`
	);

	await page.mouse.move(5, 5);
	await frames(page);
	await tick(page);
	await arrive(page, 1);
	const released = await snapshot(page);

	assert(
		released.pendingAdded === 0 && released.rendered.length === released.buffered.length,
		`with the pointer gone the withheld arrivals entered (${released.rendered.length} rendered, ` +
			`${released.buffered.length} buffered, 0 pending)`,
		`after the pointer left, ${released.pendingAdded} arrivals are still withheld ` +
			`(${released.rendered.length} rendered vs ${released.buffered.length} buffered) — the freeze latched`
	);
	await page.close();
}

/* =============================================================================
 * 5. ⚠️ THE ONE THAT GOES RED AGAINST THE PRE-FIX CODE.
 *
 *    A re-sort MOVES every row's DOM element, and the browser blurs an element
 *    that is moved — `focusout` fires from INSIDE the mutation, which is inside
 *    Svelte's flush, which is inside the keyed `{#each}`'s BLOCK_EFFECT. That
 *    is the one window in which `set()` raises `state_unsafe_mutation` and the
 *    assignment is thrown away rather than deferred.
 *
 *    The write that is lost is `focusWithin = false`. Focus is genuinely gone —
 *    `document.activeElement` is `<body>`, nobody is aiming at anything — and
 *    the list must go back to re-sorting live. With the write discarded it
 *    never does, for the rest of the session: every later arrival is withheld
 *    from a user who is not there, behind a control they have no reason to
 *    press. The freeze latches ON, which is the failure mode that looks like
 *    nothing being wrong.
 * ========================================================================== */

head('5. A re-sort that moves the focused row must not latch the freeze on');
{
	const page = await freshPage();
	await arrive(page, 8);
	await tick(page);

	const focusedKey = await page.evaluate(() => {
		const row = document.querySelectorAll(window.__freeze.rowSelector)[3];
		row.focus();
		return row.dataset.key;
	});
	await tick(page);
	note(`focus on ${focusedKey}`);

	// The explicit re-sort, with the direction flipped: every rendered row's
	// element moves. Clause 6 — direction is an input, and flipping it is the
	// ordinary way a user asks for one.
	const orderBefore = (await snapshot(page)).rendered;
	await page.evaluate(() =>
		window.__freeze.flush(() => {
			window.__freeze.feed.flipDirection();
			window.__freeze.handle.order.resort();
		})
	);
	const resorted = await snapshot(page);
	note(
		`boundary events during the re-sort (* = inside the flush): ${resorted.observed.join(', ') || 'none'}`
	);
	note(`focus is now on ${resorted.activeKey}, order.aimed is ${resorted.aimed}`);
	// Reported rather than asserted on. An aim flag can be lost without a throw,
	// and a control that keyed on the message would be asserting on Svelte's
	// wording; but when this scenario goes red the reason is almost always here.
	note(
		`uncaught page errors: ${resorted.errors.length === 0 ? 'none' : resorted.errors.join(' | ')}`
	);

	assert(
		resorted.rendered[0] === orderBefore[orderBefore.length - 1],
		'the explicit re-sort reversed the rendered order, so every row element moved',
		`the re-sort did not move the rows (head ${orderBefore[0]} → ${resorted.rendered[0]}) — ` +
			`nothing was moved out from under focus, so this scenario tested nothing`
	);
	assert(
		resorted.activeKey === 'BODY',
		'the browser blurred the moved row: focus is on <body>, so nobody is aiming any more',
		`focus survived the move (now ${resorted.activeKey}) — this browser does not blur a moved ` +
			`element, and this scenario can no longer reach the window it exists to cover`
	);

	// A task boundary, so the shipped fix's deferred write has landed.
	await tick(page);

	/* The arrival outranks everything, and the direction is now ASCENDING, so
	 * live it lands at the FOOT rather than the head. What matters is that it
	 * entered the rendered list at all, so the assertion is on membership and
	 * on the withheld count — not on which end it came in at, which would make
	 * the control depend on the direction the previous step happened to leave. */
	const renderedBefore = resorted.rendered.length;
	await arriveAbove(page);
	const after = await snapshot(page);

	assert(
		after.rendered.length === renderedBefore + 1 &&
			after.pendingAdded === 0 &&
			after.rendered.length === after.buffered.length,
		`with focus gone the list went live again: the arrival entered without being asked for ` +
			`(${renderedBefore} → ${after.rendered.length} rows, 0 withheld)`,
		`THE FREEZE LATCHED ON. Focus is on <body> and nobody is aiming, but the arrival was ` +
			`withheld (${after.rendered.length} rendered vs ${after.buffered.length} buffered, ` +
			`${after.pendingAdded} pending, control reads "${after.pending}"). The focusout that ` +
			`fired inside the flush was discarded rather than deferred` +
			(after.errors.length > 0 ? ` — uncaught: ${after.errors.join(' | ')}` : '')
	);
	await page.close();
}

/* --- done ----------------------------------------------------------------- */

await browser.close();
server.close();
await rm(outDir, { recursive: true, force: true });

process.stdout.write(
	failures === 0
		? '\n[32mfreeze-check: all scenarios passed[0m\n'
		: `\n[31mfreeze-check: ${failures} failed[0m\n`
);
process.exit(failures === 0 ? 0 : 1);
