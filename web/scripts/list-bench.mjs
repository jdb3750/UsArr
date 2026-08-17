/**
 * =============================================================================
 * THE LIST PRIMITIVE'S MEASUREMENT HARNESS — and its ARIA/roving assertions.
 * =============================================================================
 *
 * ADR-0029 says, twice and in two documents, that `contain-intrinsic-size` "has
 * no value yet" and that ARCHITECTURE §4.5 is "a direction, not an implementable
 * rule" until this measurement exists. This is that measurement.
 *
 * WHAT IT MEASURES, and why each one is here rather than assumed:
 *
 *   1. THAT CONTAINMENT IS LIVE ON THE PRIMITIVE. `content-visibility: auto` is
 *      defined entirely in terms of size, layout and paint containment, and CSS
 *      Containment Level 2 excludes internal table boxes from all three. On a
 *      `display: table-row` the declaration parses, reads back as "auto", and
 *      does nothing. A computed style echoing the input is not evidence, so the
 *      test is a scrollHeight difference — WITH a control run that forces the
 *      rows back to `display: table-row` and must produce no difference at all.
 *      Without the control the assertion has no teeth.
 *
 *   2. A REAL `contain-intrinsic-size`, derived from the rendered content-box
 *      height at each density rather than from `--row-h`. Three things the old
 *      prescription got wrong: `--row-h` was inert on the element it described,
 *      real rows are not one height, and `contain-intrinsic-size` sizes the
 *      CONTENT box so padding and border are added on top.
 *
 *   3. THE DENSITY AND THEME TOGGLES, which ADR-0029 establishes are the
 *      expensive operations rather than scrolling, and which are Tier 0 by
 *      DESIGN-DIRECTION §7.2 — hard fail at 100 ms. Measured four ways, because
 *      the ADR proposes two mitigations and does not know which one pays:
 *      containment live vs forced off, crossed with the density attribute
 *      scoped to the list container vs set on <html>.
 *
 *   4. THE DOM-ROW CEILING that sets the "Load more" page size, by sweeping row
 *      counts until the toggle crosses 100 ms.
 *
 *   5. ARROW-KEY TRAVERSAL at 25,000 rows, sibling walk vs the full rescan it
 *      replaced.
 *
 * AND IT ASSERTS THE INVARIANTS, because a number nobody checks is a number.
 * `vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so a rune
 * component cannot be imported by the unit-test run at all; the pure helpers in
 * src/lib/list.ts are tested there and everything that needs a DOM is asserted
 * here. This script exits non-zero on any failed assertion.
 *
 * ⚠️ EVERY SIZED SECTION GETS A FRESH RENDERER, and that is load-bearing rather
 * than hygienic — see `recyclePage` below. Rebuilding the corpus in place leaks
 * monotonically and the full run dies in section 5 before section 6 is ever
 * reached, which is why the 25,000-row traversal figure went unobtained for so
 * long. If a size defeats the machine anyway, the run names THAT SIZE and the
 * largest one it did complete, rather than surfacing an OOM as a mystery.
 *
 * Run: PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers pnpm bench:list
 *      pnpm bench:list -- --quick     (skip the 25k sweep; ~40s instead of ~3m)
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
const harnessDir = join(here, 'harness');
const outDir = join(tmpdir(), `usarr-list-bench-${process.pid}`);

const QUICK = process.argv.includes('--quick');
const ASSERT_ONLY = process.argv.includes('--assert-only');

/* --- reporting ------------------------------------------------------------ */

let failures = 0;
const okMark = '  [32mok[0m ';
const failMark = '  [31mFAIL[0m ';

function head(text) {
	process.stdout.write(`\n[1m${text}[0m\n`);
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
	resolve: {
		alias: {
			// The harness builds the REAL component out of src/lib. It does not
			// carry a copy: a measurement of a copy measures the copy.
			$lib: join(webDir, 'src', 'lib'),
			// SvelteKit's module, which does not exist outside a SvelteKit build.
			'$app/environment': join(harnessDir, 'app-environment.ts')
		}
	},
	plugins: [svelte({ configFile: false, preprocess: vitePreprocess() })],
	build: { outDir, emptyOutDir: true, target: 'esnext', minify: false }
});

/* --- serve it ------------------------------------------------------------- */

const MIME = {
	'.html': 'text/html; charset=utf-8',
	'.js': 'text/javascript; charset=utf-8',
	'.css': 'text/css; charset=utf-8',
	'.woff2': 'font/woff2',
	'.svg': 'image/svg+xml'
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
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
let page = await context.newPage();
await page.goto(URL_BASE);
await page.waitForSelector('table.tbl tbody tr');

/* ⚠️ TWO MEASUREMENT TRAPS THAT LOOK EXACTLY LIKE LAYOUT REGRESSIONS AND ARE
 * NEITHER. Written down here because both were raised against numbers this file
 * already published, and the answer differs between them.
 *
 *   1. CONTAINED ROWS REPORT THEIR PLACEHOLDER, NOT THEIR HEIGHT. With
 *      `content-visibility: auto` live, which rows get skipped depends on
 *      render history rather than on anything stable, so a sweep over 2,000
 *      rows can silently read `contain-intrinsic-size` back out of itself.
 *      ✅ GUARDED, deliberately and from the start: intrinsicAt() adopts a
 *      stylesheet forcing `content-visibility: visible !important` on every
 *      row, forces a layout, measures, then removes it. The guard is also
 *      self-evidencing — the harness prints the full distribution per density,
 *      so a placeholder mixed into the sample would appear as a second cluster
 *      rather than have to be reasoned about.
 *
 *   2. A FIXED SETTLE TIME CAN MEASURE PRE-WEBFONT METRICS. `document.fonts
 *      .ready` is the fix, and it is awaited below.
 *      ⚠️ BUT THE REAL SITUATION HERE IS NOT A RACE, IT IS A CONSTANT, AND
 *      THAT IS WORSE IN ONE DIRECTION AND BETTER IN ANOTHER. The harness is
 *      its own Vite root with NO `publicDir`, so app.css's `@font-face`
 *      `src: url('/fonts/PlexSans-var-latin.woff2')` resolves under the
 *      harness's own output directory, where it does not exist, and 404s. IBM
 *      Plex therefore never loads here at all: every number this file has ever
 *      printed is on the fallback face, reproducibly rather than racily, which
 *      is why the distributions have zero spread.
 *      ✅ MEASURED, AND IT DOES NOT MOVE THE NUMBERS. The rich-row figures come
 *      out at 45/49/53 byte-identical with IBM Plex served and with it blocked,
 *      checked by a canvas advance-width probe rather than assumed, and the row
 *      `line-height` is a fixed 18px length rather than a unitless multiplier.
 *      The null result is trustworthy rather than merely convenient because the
 *      same probe DOES split the two conditions once `line-height: normal` is
 *      forced, so it can detect a face-driven change and the shipped
 *      configuration simply has none. Both halves belong in this note: "these
 *      are fallback metrics" without "and here is why that does not move them"
 *      is the kind of caveat that gets the work redone.
 *      The earlier text here said instead that one-line rows were "pinned by
 *      `min-height: var(--row-h)`" and that the rich row was the one at risk.
 *      Both halves were wrong, and see the warning above about how they got in.
 *      🔍 The await and the report below change no measurement today (a 404
 *      makes fonts.ready resolve immediately); they exist so the next reader
 *      sees the font state in the output instead of assuming it was handled.
 *
 *   ⚠️ AND THE TELL THAT BOTH OF TODAY'S WRONG EXPLANATIONS SHARED: each one
 *      accounted for its number to the pixel on the first try. `min-height:
 *      var(--row-h)` explaining a 28/32/36 result is an exact fit, and it was
 *      false — the floor was slack by 1px and the match was arithmetic
 *      coincidence. A mechanism that lands exactly deserves MORE suspicion
 *      rather than less, because a mechanism and a coincidence are
 *      indistinguishable from a single agreeing measurement, and the way to
 *      tell them apart is to perturb the mechanism and see whether the number
 *      follows. Which is what `floorBinding` below now does on every run,
 *      rather than leaving the claim in prose where nothing can check it. */
await page.evaluate(() => document.fonts.ready);
const fontState = await page.evaluate(() => {
	const faces = [...document.fonts].map((f) => `${f.family} ${f.weight} ${f.status}`);
	return { count: document.fonts.size, status: document.fonts.status, faces };
});

/** Rebuild the corpus at `n` rows and let Svelte flush before measuring. */
async function setRows(n) {
	await page.evaluate((count) => {
		window.__harness.harness.setRows(count);
		window.__harness.flush();
	}, n);
	await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => r(null))));
}

/* --- the renderer's lifetime, which is what makes a full run finish --------- */

/* ⚠️ A FRESH PAGE PER SIZE. NOT TIDINESS — WITHOUT IT THE RUN DOES NOT FINISH.
 * Rebuilding the corpus in place with setRows() does not give the renderer's
 * memory back. RSS climbs monotonically across the entire run and never
 * returns: measured on this machine at 3.1 GB by section 2b and 8.5 GB by
 * section 4, after which section 5's 25,000-row sweep dies as `page.evaluate:
 * Target page, context or browser has been closed`.
 *
 * THE LEVER IS THE RENDERER'S LIFETIME, NOT THE ROW SHAPE. A cheaper row only
 * moves the size at which the same monotone climb runs out of room, so it buys
 * one more section and hides the cause. Closing the page between sizes returns
 * the memory, and peak RSS becomes one size's worth instead of the running sum
 * of every size measured so far.
 *
 * The context and the browser are kept: only the page is recycled, so the
 * measurements stay inside one browser process tree and stay comparable. */
async function recyclePage() {
	await page.close();
	page = await context.newPage();
	await page.goto(URL_BASE);
	await page.waitForSelector('table.tbl tbody tr');
	await page.evaluate(() => document.fonts.ready);
}

/** Total RSS of the browser's processes, in GB, or null off Linux. */
async function browserRssGb() {
	try {
		const { readdir, readFile } = await import('node:fs/promises');
		let pages = 0;
		for (const pid of await readdir('/proc')) {
			if (!/^\d+$/.test(pid)) continue;
			try {
				const comm = await readFile(`/proc/${pid}/comm`, 'utf8');
				if (!/headless_shell|chrome/i.test(comm)) continue;
				pages += Number((await readFile(`/proc/${pid}/statm`, 'utf8')).split(' ')[1]);
			} catch {
				/* the process exited mid-scan; it is not holding memory either way */
			}
		}
		return pages === 0 ? null : (pages * 4096) / 1024 ** 3;
	} catch {
		return null;
	}
}

/* ⚠️ AN EXPLICIT, STATED CEILING RATHER THAN AN OOM. If the renderer dies at a
 * size anyway — a smaller machine, a heavier row, a future section — the run
 * must say WHICH size defeated it and what the largest size it did complete
 * was. Dying on `Target page, context or browser has been closed` deep inside
 * an evaluate reports a Playwright message that names nothing, and reads like a
 * broken bench rather than a machine limit. An honest ceiling beats a red gate
 * whose redness means nothing. */
let ceilingHit = null;
let largestCompleted = 0;

function rendererGone(err) {
	return /target (page|closed)|has been closed|browser has been closed|crash/i.test(
		String(err?.message ?? err)
	);
}

/**
 * Run `body` at `n` rows on a FRESH renderer. Returns `body`'s value, or null
 * if the renderer died — in which case the ceiling is recorded and reported,
 * and the caller stops escalating rather than retrying into the same wall.
 */
async function atSize(n, what, body) {
	if (ceilingHit) return null;
	await recyclePage();
	try {
		await setRows(n);
		const value = await body();
		largestCompleted = Math.max(largestCompleted, n);
		return value;
	} catch (err) {
		if (!rendererGone(err)) throw err;
		ceilingHit = { n, what };
		fail(
			`${what}: the renderer died at ${n.toLocaleString()} rows. This is this machine's ` +
				`bench ceiling, not a regression in the primitive — the largest size completed in ` +
				`this run was ${largestCompleted.toLocaleString()} rows. Every figure at or above ` +
				`${n.toLocaleString()} rows in the sections below is UNOBTAINED, not passing.`
		);
		try {
			await recyclePage();
		} catch {
			/* the browser itself is gone; the summary still reports the ceiling */
		}
		return null;
	}
}

/* =============================================================================
 * 1. Containment is live on the row primitive
 * ========================================================================== */

head('1. content-visibility actually does something on this row (5,000 rows)');
await setRows(5000);

const containment = await page.evaluate(() => {
	const tbl = document.querySelector('table.tbl');
	const first = tbl.querySelector('tbody tr');

	/* SNAPSHOT, not the live declaration. getComputedStyle returns a LIVE
	 * CSSStyleDeclaration: reading a property off it at `return` time reports
	 * the state AFTER this function's own cleanup, not the state it was asked
	 * about. That is how an earlier run of this script reported the shipped
	 * contain-intrinsic-size as the stylesheet's fallback — it had removed the
	 * component's own --row-ci two lines earlier and then read the same object.
	 * Nothing about the primitive was wrong; the measurement was. */
	const cs = getComputedStyle(first);
	const shipped = {
		display: cs.display,
		contentVisibility: cs.contentVisibility,
		containIntrinsicSize: cs.containIntrinsicSize,
		rowCi: getComputedStyle(tbl).getPropertyValue('--row-ci').trim()
	};

	const sheet = new CSSStyleSheet();
	document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];

	const h = () => {
		void document.documentElement.offsetHeight;
		return document.documentElement.scrollHeight;
	};

	/* A DELIBERATELY WRONG placeholder — 200px against a ~28px row — so the
	 * signal is unambiguous. Measuring the shipped estimate here instead would
	 * pass or fail on how good the estimate is, which is a different question;
	 * the drift measurement below owns that one. This one owns only "does the
	 * declaration do anything at all". */
	tbl.style.setProperty('--row-ci', '200px');

	sheet.replaceSync('');
	const contained = h();
	sheet.replaceSync('.tbl tbody tr { content-visibility: visible !important; }');
	const uncontained = h();

	/* THE CONTROL. css-contain-2 excludes an internal table box from size,
	 * layout and paint containment, so forcing display:table-row must make the
	 * same manipulation produce EXACTLY no difference. If it ever does, this
	 * assertion has stopped measuring containment. */
	sheet.replaceSync(
		'.tbl tbody tr { display: table-row !important; content-visibility: auto !important; }'
	);
	const ctlContained = h();
	sheet.replaceSync(
		'.tbl tbody tr { display: table-row !important; content-visibility: visible !important; }'
	);
	const ctlUncontained = h();
	const ctlDisplay = getComputedStyle(first).display;

	sheet.replaceSync('');
	// Put the component's own value back rather than removing the property: the
	// component only rewrites it when one of its inputs changes, so a bare
	// removeProperty() would leave every later measurement on the stylesheet
	// fallback.
	tbl.style.setProperty('--row-ci', shipped.rowCi);
	document.adoptedStyleSheets = document.adoptedStyleSheets.filter((s) => s !== sheet);

	return {
		...shipped,
		rows: tbl.querySelectorAll('tbody tr').length,
		contained,
		uncontained,
		ctlDisplay,
		ctlContained,
		ctlUncontained
	};
});

assert(
	containment.contentVisibility === 'auto',
	`the shipped row declares content-visibility:${containment.contentVisibility}, display:${containment.display}`,
	`the shipped row primitive does not declare content-visibility:auto (computed "${containment.contentVisibility}")`
);
assert(
	containment.contained !== containment.uncontained,
	`LIVE: ${containment.rows} rows, document scrollHeight ${containment.contained}px contained vs ` +
		`${containment.uncontained}px uncontained (Δ ${Math.abs(containment.contained - containment.uncontained)}px) ` +
		`against a deliberately wrong 200px placeholder`,
	`INERT: display:${containment.display}, ${containment.rows} rows, scrollHeight ${containment.contained}px ` +
		`either way against a 200px placeholder. This is the <tr> mistake.`
);
assert(
	containment.ctlContained === containment.ctlUncontained,
	`CONTROL: the same ${containment.rows} rows forced to display:${containment.ctlDisplay} give ` +
		`${containment.ctlContained}px either way (Δ 0px), so the assertion above has teeth`,
	`CONTROL FAILED: forcing display:table-row still changed scrollHeight ` +
		`(${containment.ctlContained} vs ${containment.ctlUncontained}) — this is no longer measuring containment`
);
assert(
	containment.rowCi !== '',
	`the component wrote --row-ci through the CSSOM: "${containment.rowCi}", so the row's ` +
		`contain-intrinsic-size resolves to "${containment.containIntrinsicSize}" rather than to the ` +
		`stylesheet's fallback`,
	`--row-ci is empty, so contain-intrinsic-size fell back to the stylesheet default ` +
		`("${containment.containIntrinsicSize}") — the CSSOM write did not take`
);

/* =============================================================================
 * 2. A real contain-intrinsic-size, per density
 * ========================================================================== */

head('2. contain-intrinsic-size, derived from the rendered content box');

/**
 * Two row shapes are measured, because "the content-box height of a row" is not
 * one number and pretending it is was one of the three things ADR-0029 records
 * the old prescription getting wrong.
 *
 *   ONE-LINE   text-only cells. This is what the DEFAULT in src/lib/list.ts
 *              describes, and it is the value a list gets when it does not pass
 *              `rowIntrinsic`.
 *   RICH       this harness's real release row — chips, a button, a checkbox
 *              and a <select>. A list like that passes its own value; a 32px
 *              control alone puts its floor above every density's --row-h.
 */
async function measureIntrinsic(shape) {
	await page.evaluate((s) => {
		window.__harness.harness.setSimple(s === 'one-line');
		window.__harness.flush();
	}, shape);
	const out = {};
	for (const density of ['compact', 'standard', 'relaxed'])
		out[density] = await intrinsicAt(density);
	return out;
}

async function intrinsicAt(density) {
	return page.evaluate((d) => {
		window.__harness.prefs.setDensity(d);
		window.__harness.flush();

		const tbl = document.querySelector('table.tbl');
		const sheet = new CSSStyleSheet();
		document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
		// Every row must actually lay out, or an off-screen row reports its
		// placeholder height and the measurement measures its own input.
		sheet.replaceSync('.tbl tbody tr { content-visibility: visible !important; }');
		void tbl.offsetHeight;

		const rows = [...tbl.querySelectorAll('tbody tr')].slice(0, 2000);
		const cs = getComputedStyle(rows[0]);
		const padY = parseFloat(cs.paddingTop) + parseFloat(cs.paddingBottom);
		const borderY = parseFloat(cs.borderTopWidth) + parseFloat(cs.borderBottomWidth);

		const counts = new Map();
		let sum = 0;
		for (const row of rows) {
			// The row is the border box; contain-intrinsic-size sizes the CONTENT
			// box, so padding and border come off.
			const content = Math.round((row.getBoundingClientRect().height - padY - borderY) * 10) / 10;
			counts.set(content, (counts.get(content) ?? 0) + 1);
			sum += content;
		}

		// WHICH REGIME PRODUCED THOSE NUMBERS, MEASURED RATHER THAN ASSERTED.
		//
		// `.tbl tbody tr` carries `min-height: var(--row-h)`, and whether that
		// floor is what SETS the row height or is merely present is not visible
		// in the height itself: a natural height above the floor and a natural
		// height below it both render as a number, and only one of them is the
		// floor's doing. The two cases have been confused in this file's own
		// prose in both directions, so the run decides it now. Drop the floor,
		// force a layout, re-measure: if the height moves, the floor was
		// binding; if it does not, the floor is slack and the figures above are
		// the content's own.
		sheet.replaceSync(
			'.tbl tbody tr { content-visibility: visible !important; min-height: 0 !important; }'
		);
		void tbl.offsetHeight;
		const naturalCounts = new Map();
		for (const row of rows) {
			const content = Math.round((row.getBoundingClientRect().height - padY - borderY) * 10) / 10;
			naturalCounts.set(content, (naturalCounts.get(content) ?? 0) + 1);
		}

		sheet.replaceSync('');
		document.adoptedStyleSheets = document.adoptedStyleSheets.filter((s) => s !== sheet);

		const distinct = [...counts.entries()].sort((a, b) => a[0] - b[0]);
		const naturalDistinct = [...naturalCounts.entries()].sort((a, b) => a[0] - b[0]);
		const mode = distinct.reduce((a, b) => (b[1] > a[1] ? b : a))[0];
		const naturalMode = naturalDistinct.reduce((a, b) => (b[1] > a[1] ? b : a))[0];
		return {
			rowH: parseFloat(getComputedStyle(tbl).getPropertyValue('--row-h')),
			padY,
			borderY,
			n: rows.length,
			distinct,
			min: distinct[0][0],
			max: distinct[distinct.length - 1][0],
			mean: Math.round((sum / rows.length) * 10) / 10,
			mode,
			/** The content box with `min-height` forced to 0: the content's own height. */
			naturalMode,
			/** True when dropping the floor changed the height, i.e. the floor was load-bearing. */
			floorBinding: naturalMode < mode
		};
	}, density);
}

const shapes = {};
for (const shape of ['one-line', 'rich']) {
	shapes[shape] = await measureIntrinsic(shape);
	for (const density of ['compact', 'standard', 'relaxed']) {
		const m = shapes[shape][density];
		ok(
			`${shape.padEnd(8)} ${density.padEnd(9)} --row-h=${m.rowH}px  padding=${m.padY}px  ` +
				`border=${m.borderY}px  content box over n=${m.n}: min=${m.min} mode=${m.mode} ` +
				`mean=${m.mean} max=${m.max}, ${m.distinct.length} distinct heights`
		);
		note(`  distribution: ${m.distinct.map(([h, c]) => `${h}px×${c}`).join(', ')}`.slice(0, 220));
	}
}
await page.evaluate(() => {
	window.__harness.harness.setSimple(false);
	window.__harness.prefs.setDensity('compact');
	window.__harness.flush();
});
const intrinsic = shapes.rich;

/* The recommendation, and the arithmetic behind it. `auto` in front of the
 * length means the browser replaces the estimate with the row's real size once
 * it has seen the row, so this number only governs rows that have NEVER been on
 * screen — which makes the mean the right statistic (it minimises total
 * placeholder error over the unseen tail) rather than the mode or the floor. */
const recommended = {};
const recommendedOneLine = {};
for (const d of ['compact', 'standard', 'relaxed']) {
	recommended[d] = Math.round(shapes.rich[d].mean);
	recommendedOneLine[d] = Math.round(shapes['one-line'][d].mean);
}
note(
	`ROW_INTRINSIC default (one-line row, mean content box): ` +
		Object.entries(recommendedOneLine)
			.map(([d, v]) => `${d} ${v}px`)
			.join(' · ')
);
note(
	`this list's own rowIntrinsic (rich row, mean content box): ` +
		Object.entries(recommended)
			.map(([d, v]) => `${d} ${v}px`)
			.join(' · ')
);

/* --- the gate on that value: scrollbar drift under 2% --------------------- */

/**
 * One drift run. `fresh` destroys and rebuilds every row element first, which
 * is what makes the run measure the value under test rather than the previous
 * run's remembered sizes — see the finding printed after this block.
 */
async function driftRun(density, ci, fresh) {
	return page.evaluate(
		async ([d, value, rebuild]) => {
			window.__harness.harness.setRowIntrinsic(value);
			window.__harness.prefs.setDensity(d);
			window.__harness.flush();
			if (rebuild) {
				window.__harness.harness.regenerate();
				window.__harness.flush();
			}
			window.scrollTo(0, 0);
			await new Promise((r) => requestAnimationFrame(r));
			void document.documentElement.offsetHeight;
			const atLoad = document.documentElement.scrollHeight;

			/* A full scroll in viewport-sized steps, so every row is rendered at
			 * least once and `auto` replaces every placeholder with the row's real
			 * height. Steps are viewport-sized rather than larger because a larger
			 * step would skip rows, which is the one thing this measurement cannot
			 * do.
			 *
			 * IT AWAITS A FRAME PER STEP, AND THAT IS NOT AN OPTIMISATION WAITING
			 * TO HAPPEN. The obvious speed-up — scroll and force layout with a
			 * scrollHeight read, no frame — was tried, because ~250 awaited frames
			 * on a 5,000-row document is minutes. It is wrong. Relevance for
			 * `content-visibility: auto` is determined at a RENDERING OPPORTUNITY,
			 * not during a forced layout, so the skipped rows never render and
			 * `auto` never learns their real size. Measured, 5,000 rows at compact:
			 * the frame-per-step loop reports 0.76% drift, the forced-layout loop
			 * reports 0.01%. The second number is the measurement failing to
			 * happen, not the drift going away. It is slow because the work is
			 * real. */
			const step = window.innerHeight;
			for (let y = 0; y < document.documentElement.scrollHeight; y += step) {
				window.scrollTo(0, y);
				await new Promise((r) => requestAnimationFrame(r));
			}
			void document.documentElement.offsetHeight;
			const afterScroll = document.documentElement.scrollHeight;
			window.scrollTo(0, 0);
			return { atLoad, afterScroll, drift: Math.abs(afterScroll - atLoad) / afterScroll };
		},
		[density, ci, fresh]
	);
}

head('2b. Scrollbar drift with the measured value (ADR-0029 gate: < 2%)');
await setRows(5000);
for (const density of ['compact', 'standard', 'relaxed']) {
	const drift = await driftRun(density, recommended[density], true);
	const pct = (drift.drift * 100).toFixed(2);
	assert(
		drift.drift < 0.02,
		`${density.padEnd(9)} rowIntrinsic=${recommended[density]}px over 5,000 fresh rows: ` +
			`${drift.atLoad}px at load, ${drift.afterScroll}px after a full scroll, drift ${pct}%`,
		`${density.padEnd(9)} rowIntrinsic=${recommended[density]}px: drift ${pct}% exceeds the 2% budget ` +
			`(${drift.atLoad}px → ${drift.afterScroll}px)`
	);
}

head('2c. A `contain-intrinsic-size: auto` behaviour the drift test exposed');
/* NOT A FAILURE, AND NOT IN ANY OF THE THREE DOCUMENTS — reported because it is
 * the one place where the shipped configuration is measurably worse than the
 * gate suggests, and because it is invisible unless you look for it.
 *
 * `auto` means "use the length until you have rendered this element, then
 * remember what it actually was". The memory belongs to the ELEMENT and it
 * survives the element going off screen again. A keyed {#each} reuses the same
 * DOM nodes across a density change by design, so after a density change every
 * off-screen row is still described by the height it had at the OLD density,
 * and the scrollbar is wrong by that difference until the user scrolls through.
 * The measurement below is the same 5,000 rows, scrolled once at compact and
 * then switched to relaxed without rebuilding. */
{
	await setRows(5000);
	await driftRun('compact', recommended.compact, true);
	const stale = await driftRun('relaxed', recommended.relaxed, false);
	const fresh = await driftRun('relaxed', recommended.relaxed, true);
	note(
		`reused rows after compact → relaxed: ${stale.atLoad}px at load vs ${stale.afterScroll}px ` +
			`after scrolling, drift ${(stale.drift * 100).toFixed(2)}%`
	);
	note(
		`the same rows rebuilt at relaxed:    ${fresh.atLoad}px at load vs ${fresh.afterScroll}px ` +
			`after scrolling, drift ${(fresh.drift * 100).toFixed(2)}%`
	);
	note(
		`so the remembered size, not the declared one, is what a post-density-change scrollbar is ` +
			`built from. It self-corrects as the user scrolls and it costs nothing to leave alone; ` +
			`what it rules out is any claim that the drift budget holds across a density change.`
	);
	await page.evaluate(() => {
		window.__harness.harness.setRowIntrinsic(undefined);
		window.__harness.prefs.setDensity('compact');
		window.__harness.flush();
	});
}

/* =============================================================================
 * 3. The density and theme toggles — the expensive operations
 * ========================================================================== */

/**
 * One toggle measurement. `scope` decides whether the attribute lands on <html>
 * (what the application does today for everything else) or on the list
 * container (ADR-0029's second mitigation). `containment` false reproduces the
 * condition the ADR's 153/1,199/6,508 ms figures were taken under.
 */
async function toggleCost(page, { attribute, values, scope, containment }) {
	return page.evaluate(
		([attr, vals, sc, cont]) => {
			const tbl = document.querySelector('table.tbl');
			const sheet = new CSSStyleSheet();
			document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
			if (!cont) sheet.replaceSync('.tbl tbody tr { content-visibility: visible !important; }');

			// In root scope the list must not carry its own attribute, or its own
			// value wins over the inherited one and nothing changes.
			const own = tbl.getAttribute(attr);
			if (sc === 'root') tbl.removeAttribute(attr);

			const target = sc === 'root' ? document.documentElement : tbl;
			const restoreRoot = document.documentElement.getAttribute(attr);

			void document.documentElement.offsetHeight;
			const samples = [];
			for (let i = 0; i < 4; i++) {
				const v = vals[i % vals.length];
				const t0 = performance.now();
				target.setAttribute(attr, v);
				// Force style recalculation AND layout, which is what the user waits
				// for. performance.now() around the setAttribute alone measures
				// nothing — the work is deferred to the next style pass.
				void document.documentElement.offsetHeight;
				void tbl.scrollHeight;
				samples.push(performance.now() - t0);
			}

			if (sc === 'root') {
				if (restoreRoot === null) document.documentElement.removeAttribute(attr);
				else document.documentElement.setAttribute(attr, restoreRoot);
				if (own !== null) tbl.setAttribute(attr, own);
			}
			sheet.replaceSync('');
			document.adoptedStyleSheets = document.adoptedStyleSheets.filter((s) => s !== sheet);

			const mean = samples.reduce((a, b) => a + b, 0) / samples.length;
			return { mean: Math.round(mean * 10) / 10, samples: samples.map((s) => Math.round(s)) };
		},
		[attribute, values, scope, containment]
	);
}

const SIZES = QUICK ? [1000, 5000] : [1000, 5000, 25000];
const DENSITY_VALUES = ['standard', 'relaxed', 'compact', 'standard'];
const THEME_VALUES = ['dark', 'light', 'dark', 'light'];

head('3. Density toggle (Tier 0 by §7.2 — hard fail 100 ms), mean of four changes');
note('columns: containment live / containment forced off, x  list-scoped / root-scoped');
const densityTable = [];
for (const n of SIZES) {
	const row = await atSize(n, 'section 3 density toggle', async () => {
		const row = { n };
		for (const cont of [true, false]) {
			for (const scope of ['container', 'root']) {
				const r = await toggleCost(page, {
					attribute: 'data-density',
					values: DENSITY_VALUES,
					scope,
					containment: cont
				});
				row[`${cont ? 'cv' : 'nocv'}_${scope}`] = r.mean;
			}
		}
		return row;
	});
	if (!row) break;
	densityTable.push(row);
	/* A MEASUREMENT, NOT AN ASSERTION, and the distinction is the point of the
	 * whole exercise. 25,000 rows in the DOM is ABOVE the ceiling this section
	 * exists to find, so failing on it would be asserting that the ceiling does
	 * not exist. The assertion is the one below: that the page size derived from
	 * these numbers keeps a real session under the hard fail. */
	note(
		`${String(n).padStart(6)} rows   ` +
			`cv+list ${String(row.cv_container).padStart(7)} ms   ` +
			`cv+root ${String(row.cv_root).padStart(7)} ms   ` +
			`nocv+list ${String(row.nocv_container).padStart(7)} ms   ` +
			`nocv+root ${String(row.nocv_root).padStart(7)} ms` +
			(row.cv_container > 100 ? '   <-- above the 100 ms Tier-0 hard fail' : '')
	);
}
const rssAfter3 = await browserRssGb();
if (rssAfter3 !== null)
	note(
		`browser RSS after section 3: ${rssAfter3.toFixed(2)} GB — reported, not asserted, ` +
			`because the fresh-page-per-size recycling is the only reason this run reaches section 5`
	);

head('4. Theme toggle, mean of four changes');
for (const n of SIZES) {
	const themed = await atSize(n, 'section 4 theme toggle', async () => {
		const live = await toggleCost(page, {
			attribute: 'data-theme',
			values: THEME_VALUES,
			scope: 'root',
			containment: true
		});
		const off = await toggleCost(page, {
			attribute: 'data-theme',
			values: THEME_VALUES,
			scope: 'root',
			containment: false
		});
		return { live, off };
	});
	if (!themed) break;
	note(
		`${String(n).padStart(6)} rows   containment live ${String(themed.live.mean).padStart(7)} ms   ` +
			`containment off ${String(themed.off.mean).padStart(7)} ms` +
			(themed.live.mean > 100 ? '   <-- above the 100 ms Tier-0 hard fail' : '')
	);
}
await page.evaluate(() => {
	document.documentElement.removeAttribute('data-theme');
});

/* =============================================================================
 * 5. The DOM-row ceiling that sets the "Load more" page size
 * ========================================================================== */

head('5. The DOM-row ceiling, swept against the 100 ms Tier-0 hard fail');
const SWEEP = QUICK ? [200, 800, 3200] : [100, 200, 400, 800, 1600, 3200, 6400, 12800, 25000];
const sweep = [];
for (const n of SWEEP) {
	const point = await atSize(n, 'section 5 ceiling sweep', async () => {
		const shipped = await toggleCost(page, {
			attribute: 'data-density',
			values: DENSITY_VALUES,
			scope: 'container',
			containment: true
		});
		const worst = await toggleCost(page, {
			attribute: 'data-density',
			values: DENSITY_VALUES,
			scope: 'root',
			containment: false
		});
		return { n, shipped: shipped.mean, worst: worst.mean };
	});
	if (!point) break;
	sweep.push(point);
	note(
		`${String(n).padStart(6)} rows   as shipped ${String(point.shipped).padStart(7)} ms ` +
			`(${(point.shipped / n).toFixed(4)} ms/row)   worst case ${String(point.worst).padStart(7)} ms ` +
			`(${(point.worst / n).toFixed(4)} ms/row)`
	);
}

/**
 * The crossing point, by BRACKETING rather than by a global linear fit.
 *
 * A least-squares line through the whole sweep is the obvious thing to write
 * and it is wrong here: the cost is close to linear up to a few thousand rows
 * and then bends sharply upward, so a global fit is dragged by the 25,000-row
 * point and reports a crossing that no measured pair supports. Bracketing takes
 * the last size measured UNDER 100 ms and the first measured OVER it and
 * interpolates between those two points only — which is a statement about
 * measured data rather than about a model of it.
 */
function crossing(points, field) {
	const sorted = [...points].sort((a, b) => a.n - b.n);
	let under = null;
	for (const p of sorted) {
		if (p[field] > 100) {
			if (!under) return { at100: sorted[0].n, bracket: [null, p], linear: false };
			const t = (100 - under[field]) / (p[field] - under[field]);
			return { at100: under.n + t * (p.n - under.n), bracket: [under, p], linear: false };
		}
		under = p;
	}
	// Never crossed. Extrapolate from the last two points, and say so.
	const a = sorted[sorted.length - 2];
	const b = sorted[sorted.length - 1];
	const slope = (b[field] - a[field]) / (b.n - a.n);
	return {
		at100: slope > 0 ? b.n + (100 - b[field]) / slope : Infinity,
		bracket: [a, b],
		linear: true
	};
}

/** Per-row cost between two adjacent sweep points — the marginal cost. */
function marginal(points, field) {
	const sorted = [...points].sort((a, b) => a.n - b.n);
	const out = [];
	for (let i = 1; i < sorted.length; i++) {
		out.push({
			from: sorted[i - 1].n,
			to: sorted[i].n,
			perRow: (sorted[i][field] - sorted[i - 1][field]) / (sorted[i].n - sorted[i - 1].n)
		});
	}
	return out;
}

const fitShipped = crossing(sweep, 'shipped');
const fitWorst = crossing(sweep, 'worst');
note('');
note(
	'marginal ms/row as shipped: ' +
		marginal(sweep, 'shipped')
			.map((m) => `${m.from}→${m.to} ${m.perRow.toFixed(4)}`)
			.join('  ')
);
note(
	'marginal ms/row worst case: ' +
		marginal(sweep, 'worst')
			.map((m) => `${m.from}→${m.to} ${m.perRow.toFixed(4)}`)
			.join('  ')
);
note('');
for (const [name, fit] of [
	['as shipped', fitShipped],
	['worst case', fitWorst]
]) {
	const [a, b] = fit.bracket;
	note(
		`${name.padEnd(11)} crosses 100 ms at ~${Math.round(fit.at100).toLocaleString()} rows on this ` +
			`desktop` +
			(a && b
				? ` (bracketed by ${a.n.toLocaleString()} rows and ${b.n.toLocaleString()} rows` +
					(fit.linear ? ', EXTRAPOLATED — the sweep never crossed' : ', measured on both sides') +
					')'
				: ' (the smallest size measured was already over)')
	);
	note(
		`            Pi 5 at ADR-0029's 3–5×: ${Math.round(fit.at100 / 5).toLocaleString()}–` +
			`${Math.round(fit.at100 / 3).toLocaleString()} rows`
	);
}

/* THE ASSERTION THIS WHOLE SECTION EXISTS FOR. The tables above are
 * measurements; this is the gate. LOAD_MORE_PAGE_SIZE is what a caller gets by
 * default, so the cost at that many rows must leave enough headroom under the
 * 100 ms Tier-0 hard fail to survive the Pi-5 multiplier the ADR uses. 5x is
 * the pessimistic end of that multiplier, so the desktop budget is 20 ms. */
const PAGE_SIZE = 200;
await setRows(PAGE_SIZE);
const atPageSize = await toggleCost(page, {
	attribute: 'data-density',
	values: DENSITY_VALUES,
	scope: 'container',
	containment: true
});
assert(
	atPageSize.mean <= 20,
	`one page (${PAGE_SIZE} rows): density toggle ${atPageSize.mean} ms on this desktop, which is ` +
		`${(100 / atPageSize.mean).toFixed(0)}x under the 100 ms Tier-0 hard fail — enough headroom for ` +
		`ADR-0029's 3–5x Pi 5 multiplier`,
	`one page (${PAGE_SIZE} rows): density toggle ${atPageSize.mean} ms, which leaves less than the 5x ` +
		`headroom the Pi-5 multiplier needs. LOAD_MORE_PAGE_SIZE in src/lib/list.ts is too large.`
);

/* =============================================================================
 * 6. Arrow-key traversal: sibling walk vs the rescan it replaced
 * ========================================================================== */

head('6. Arrow-key traversal at 25,000 rows');
if (!QUICK) {
	const traversal = await atSize(25000, 'section 6 arrow-key traversal', () =>
		page.evaluate(() => {
			const tbl = document.querySelector('table.tbl');
			const SEL = '[role="row"][data-key]';
			const first = tbl.querySelector('tbody tr');
			first.focus();

			// THE SHIPPED PATH: a real keydown through the roving action, which walks
			// nextElementSibling. 200 presses, which is about a second of key repeat.
			const t0 = performance.now();
			for (let i = 0; i < 200; i++) {
				document.activeElement.dispatchEvent(
					new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true })
				);
			}
			const walk = (performance.now() - t0) / 200;

			// THE PATH IT REPLACED: querySelectorAll plus indexOf, per keypress.
			let cursor = tbl.querySelector('tbody tr');
			const t1 = performance.now();
			for (let i = 0; i < 200; i++) {
				const all = [...tbl.querySelectorAll(SEL)].filter((r) => !r.hidden);
				const at = all.indexOf(cursor);
				cursor = all[at + 1] ?? all[0];
			}
			const rescan = (performance.now() - t1) / 200;

			return {
				rows: tbl.querySelectorAll('tbody tr').length,
				walk: Math.round(walk * 100) / 100,
				rescan: Math.round(rescan * 100) / 100,
				landedOn: document.activeElement.dataset.key
			};
		})
	);
	if (traversal) {
		assert(
			traversal.walk < traversal.rescan,
			`${traversal.rows.toLocaleString()} rows: sibling walk ${traversal.walk} ms/keypress vs ` +
				`rescan ${traversal.rescan} ms/keypress (${(traversal.rescan / traversal.walk).toFixed(0)}×), ` +
				`200 presses landed on ${traversal.landedOn}`,
			`the sibling walk (${traversal.walk} ms) is not faster than the rescan (${traversal.rescan} ms) — ` +
				`either the walk regressed or this is not measuring what it thinks`
		);
		note(
			`one second of held ArrowDown: ${(traversal.walk * 60).toFixed(1)} ms of main thread on the ` +
				`walk, ${(traversal.rescan * 60).toFixed(0)} ms on the rescan`
		);
	}
} else {
	note('skipped under --quick');
}

/* =============================================================================
 * 7. Append cost — "Load more" is the primary interaction
 * ========================================================================== */

head('7. "Load more" append cost, including the roving reassignment');
for (const n of QUICK ? [1000] : [1000, 5000, 25000]) {
	const append = await atSize(n, 'section 7 append cost', () =>
		page.evaluate(() => {
			const before = document.querySelectorAll('.tbl tbody tr').length;
			const t0 = performance.now();
			window.__harness.harness.loadMore(200);
			window.__harness.flush();
			void document.documentElement.offsetHeight;
			const elapsed = performance.now() - t0;
			return {
				elapsed: Math.round(elapsed * 10) / 10,
				before,
				after: document.querySelectorAll('.tbl tbody tr').length
			};
		})
	);
	if (!append) break;
	ok(
		`${String(n).padStart(6)} rows → +200: ${append.elapsed} ms (${append.before} → ${append.after} rows)`
	);
}

/* =============================================================================
 * 8. THE INVARIANTS. Everything below is an assertion, not a measurement.
 * ========================================================================== */

head('8. Roving tabindex: the list is ONE tab stop');
await setRows(300);

const tabstops = async () =>
	page.evaluate(() => {
		const tbl = document.querySelector('table.tbl');
		const rows = [...tbl.querySelectorAll('tbody tr')];
		return {
			zero: rows.filter((r) => r.tabIndex === 0).length,
			zeroKey: rows.find((r) => r.tabIndex === 0)?.dataset.key ?? null,
			innerPositive: [
				...tbl.querySelectorAll('tbody a[href], tbody button, tbody input, tbody select')
			].filter((e) => e.tabIndex >= 0).length,
			innerTotal: tbl.querySelectorAll('tbody a[href], tbody button, tbody input, tbody select')
				.length,
			rows: rows.length
		};
	});

let t = await tabstops();
assert(
	t.zero === 1,
	`at load: exactly 1 of ${t.rows} rows at tabindex=0 (${t.zeroKey})`,
	`at load: ${t.zero} rows at tabindex=0, expected exactly 1`
);
assert(
	t.innerPositive === 0,
	`at load: all ${t.innerTotal} focusable descendants are tabindex=-1, so the row is the only stop`,
	`at load: ${t.innerPositive} of ${t.innerTotal} row descendants are still tab stops`
);

// After an append — ADR-0029 makes this the primary interaction, and a tabindex
// set once at init is wrong within one click.
await page.evaluate(() => {
	window.__harness.harness.loadMore(200);
	window.__harness.flush();
});
t = await tabstops();
assert(
	t.zero === 1,
	`after a 200-row append: still exactly 1 of ${t.rows} rows at tabindex=0 (${t.zeroKey})`,
	`after a 200-row append: ${t.zero} rows at tabindex=0`
);
assert(
	t.innerPositive === 0,
	`after a 200-row append: all ${t.innerTotal} appended descendants are tabindex=-1`,
	`after a 200-row append: ${t.innerPositive} of ${t.innerTotal} descendants became tab stops`
);

head('9. The tab stop is keyed to a row IDENTITY, not to a position');
const reorder = await page.evaluate(() => {
	const tbl = document.querySelector('table.tbl');
	const rows = [...tbl.querySelectorAll('tbody tr')];
	// Arrow down to the fifth row, so the roved row is not the one a fresh
	// assignment would pick.
	rows[0].focus();
	for (let i = 0; i < 4; i++) {
		document.activeElement.dispatchEvent(
			new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true })
		);
	}
	const before = {
		focused: document.activeElement.dataset.key,
		index: [...tbl.querySelectorAll('tbody tr')].indexOf(document.activeElement)
	};

	window.__harness.harness.reverse();
	window.__harness.flush();

	const after = [...tbl.querySelectorAll('tbody tr')];
	return {
		before,
		focusedKey: document.activeElement?.dataset?.key ?? null,
		tabZeroKey: after.find((r) => r.tabIndex === 0)?.dataset.key ?? null,
		tabZeroCount: after.filter((r) => r.tabIndex === 0).length,
		newIndex: after.indexOf(document.activeElement)
	};
});
assert(
	reorder.tabZeroKey === reorder.before.focused,
	`after reversing ${t.rows} rows: the tab stop is still on ${reorder.tabZeroKey}, which moved from ` +
		`index ${reorder.before.index} to ${reorder.newIndex}. A positional key would have left it at ` +
		`index ${reorder.before.index}, on different data.`,
	`after reversing: the tab stop moved from ${reorder.before.focused} to ${reorder.tabZeroKey} — ` +
		`this is the positional-key teleport ADR-0029's identity rule exists to prevent`
);
assert(
	reorder.tabZeroCount === 1,
	`after reversing: still exactly one row at tabindex=0`,
	`after reversing: ${reorder.tabZeroCount} rows at tabindex=0`
);

head('10. aria-rowcount / aria-rowindex over the FULL set');
const aria = async () =>
	page.evaluate(() => {
		const tbl = document.querySelector('table.tbl');
		const bodyRows = [...tbl.querySelectorAll('tbody tr')];
		const idx = bodyRows.map((r) => Number(r.getAttribute('aria-rowindex')));
		let contiguous = true;
		for (let i = 1; i < idx.length; i++) if (idx[i] !== idx[i - 1] + 1) contiguous = false;
		return {
			rowcount: Number(tbl.getAttribute('aria-rowcount')),
			colcount: Number(tbl.getAttribute('aria-colcount')),
			headerIndex: Number(tbl.querySelector('thead tr').getAttribute('aria-rowindex')),
			first: idx[0],
			last: idx[idx.length - 1],
			rendered: bodyRows.length,
			contiguous,
			roles: {
				table: tbl.getAttribute('role'),
				rowgroup: tbl.querySelector('tbody').getAttribute('role'),
				row: bodyRows[0].getAttribute('role'),
				columnheader: tbl.querySelector('thead th').getAttribute('role'),
				cell: bodyRows[0].querySelector('td').getAttribute('role')
			}
		};
	});
let a = await aria();
assert(
	a.roles.table === 'table' &&
		a.roles.rowgroup === 'rowgroup' &&
		a.roles.row === 'row' &&
		a.roles.columnheader === 'columnheader' &&
		a.roles.cell === 'cell',
	`explicit roles present: table / rowgroup / row / columnheader / cell`,
	`missing explicit roles: ${JSON.stringify(a.roles)}`
);
assert(
	a.headerIndex === 1 && a.first === 2 && a.contiguous,
	`header aria-rowindex=1, data rows ${a.first}–${a.last} contiguous over ${a.rendered} rendered`,
	`aria-rowindex is wrong: header=${a.headerIndex}, first=${a.first}, contiguous=${a.contiguous}`
);
assert(
	a.rowcount > a.rendered,
	`aria-rowcount=${a.rowcount} describes the FULL set, not the ${a.rendered} rendered rows — ` +
		`without this a screen reader says "row 3 of ${a.rendered}" when the truth is "row 3 of ${a.rowcount - 1}"`,
	`aria-rowcount=${a.rowcount} equals or trails the rendered count ${a.rendered}`
);

const before = a;
await page.evaluate(() => {
	window.__harness.harness.loadMore(200);
	window.__harness.flush();
});
a = await aria();
assert(
	a.contiguous &&
		a.first === 2 &&
		a.rendered === before.rendered + 200 &&
		a.rowcount === before.rowcount,
	`after a 200-row append: indices ${a.first}–${a.last} still contiguous over ${a.rendered} rows, ` +
		`aria-rowcount unchanged at ${a.rowcount}`,
	`after a 200-row append: contiguous=${a.contiguous}, first=${a.first}, rendered=${a.rendered}, rowcount=${a.rowcount}`
);

const unknown = await page.evaluate(() => {
	window.__harness.harness.setTotal(undefined);
	window.__harness.flush();
	const v = document.querySelector('table.tbl').getAttribute('aria-rowcount');
	window.__harness.harness.setTotal(1204);
	window.__harness.flush();
	return v;
});
assert(
	unknown === '-1',
	`aria-rowcount is -1 when the total is genuinely unknown, which is what ARIA defines it for`,
	`aria-rowcount is "${unknown}" when the total is unknown, expected "-1"`
);

head('11. The roving handler does not steal keys from a form control (SC 2.1.1)');
const formBailout = await page.evaluate(() => {
	const tbl = document.querySelector('table.tbl');
	const select = tbl.querySelector('tbody select');
	const row = select.closest('[role="row"]');
	select.focus();
	const results = {};
	for (const key of ['ArrowDown', 'ArrowUp', 'Home', 'End']) {
		const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
		select.dispatchEvent(ev);
		results[key] = {
			defaultPrevented: ev.defaultPrevented,
			stillOnSelect: document.activeElement === select
		};
	}
	// Escape must be handled BEFORE the bail-out, and must return focus to the row.
	const esc = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
	select.dispatchEvent(esc);
	return {
		results,
		escapeReturnedToRow: document.activeElement === row,
		rowTabIndex: row.tabIndex,
		landedOn: document.activeElement?.tagName + '.' + (document.activeElement?.className || '')
	};
});
const stolen = Object.entries(formBailout.results).filter(
	([, r]) => r.defaultPrevented || !r.stillOnSelect
);
assert(
	stolen.length === 0,
	`ArrowDown / ArrowUp / Home / End inside a <select> are left alone — focus stays on the control ` +
		`and nothing is preventDefault()ed`,
	`the roving handler swallowed ${stolen.map(([k]) => k).join(', ')} inside a <select>, which makes ` +
		`it keyboard-inoperable — SC 2.1.1, Level A`
);
assert(
	formBailout.escapeReturnedToRow,
	`Escape from inside the row's controls returns focus to the row, and it is handled BEFORE the ` +
		`form-control bail-out that would otherwise swallow it`,
	`Escape inside a row's <select> did not return focus to the row — it landed on ` +
		`${formBailout.landedOn} (the row's own tabindex is ${formBailout.rowTabIndex})`
);

head('12. §9.1 row rules, measured on the rendered primitive');
const rowRules = await page.evaluate(() => {
	const wrap = document.querySelector('.tablewrap');
	const tbl = document.querySelector('table.tbl');
	const rows = [...tbl.querySelectorAll('tbody tr')].slice(0, 6);
	const thead = tbl.querySelector('thead');
	const theadCS = getComputedStyle(thead);
	const numCell = tbl.querySelector('tbody td.is-num');
	const numHeader = tbl.querySelector('thead th.is-num');
	const proseCell = tbl.querySelector('tbody td[data-col="release"]');
	const label = tbl.querySelector('tbody .stacklabel');
	const monoCell = tbl.querySelector('tbody td[data-col="release"] .mono');
	const chipCells = [...tbl.querySelectorAll('tbody td[data-col="flags"]')];

	const alpha = (c) => {
		const m = c.match(/rgba?\(([^)]+)\)/);
		if (!m) return null;
		const parts = m[1].split(/[,/\s]+/).filter(Boolean);
		return parts.length > 3 ? parseFloat(parts[3]) : 1;
	};

	return {
		wrapOverflowX: getComputedStyle(wrap).overflowX,
		wrapOverflowY: getComputedStyle(wrap).overflowY,
		theadPosition: theadCS.position,
		theadBg: theadCS.backgroundColor,
		theadAlpha: alpha(theadCS.backgroundColor),
		zebra: rows.map((r) => getComputedStyle(r).backgroundColor),
		rule: rows.map((r) => getComputedStyle(r).borderBottomWidth),
		numCellAlign: getComputedStyle(numCell).textAlign,
		numHeaderAlign: getComputedStyle(numHeader).textAlign,
		numCellNumeric: getComputedStyle(numCell).fontVariantNumeric,
		proseNumeric: getComputedStyle(proseCell).fontVariantNumeric,
		labelHidden: label?.getAttribute('aria-hidden') ?? null,
		labelTag: label?.tagName ?? null,
		labelBefore: label ? getComputedStyle(label, '::before').content : null,
		cellBefore: getComputedStyle(proseCell, '::before').content,
		monoWrap: monoCell ? getComputedStyle(monoCell).overflowWrap : null,
		monoWhiteSpace: monoCell ? getComputedStyle(monoCell).whiteSpace : null,
		monoTitle: monoCell ? monoCell.getAttribute('title') : null,
		monoTruncated: monoCell ? monoCell.scrollWidth > monoCell.clientWidth : null,
		maxChips: Math.max(...chipCells.map((c) => c.querySelectorAll('.chip').length)),
		cols: getComputedStyle(tbl).getPropertyValue('--cols').trim(),
		rowCi: getComputedStyle(tbl).getPropertyValue('--row-ci').trim(),
		gridTemplate: getComputedStyle(rows[0]).gridTemplateColumns
	};
});

assert(
	rowRules.wrapOverflowY === 'visible',
	`the wrapper is overflow-x:${rowRules.wrapOverflowX} / overflow-y:${rowRules.wrapOverflowY}, so it ` +
		`is not a scroll container and the sticky header can reach the viewport`,
	`the wrapper computes overflow-y:${rowRules.wrapOverflowY} — this is the auto-on-both-axes trap that ` +
		`breaks sticky headers between 761 and 1,099 px`
);
assert(
	rowRules.theadPosition === 'sticky' && rowRules.theadAlpha === 1,
	`sticky header with an OPAQUE background (${rowRules.theadBg}, alpha ${rowRules.theadAlpha})`,
	`sticky header: position=${rowRules.theadPosition}, background=${rowRules.theadBg} (alpha ${rowRules.theadAlpha})`
);
assert(
	new Set(rowRules.zebra).size === 1,
	`no zebra striping: all six sampled rows render ${rowRules.zebra[0]}`,
	`zebra striping detected: ${[...new Set(rowRules.zebra)].join(' / ')}`
);
assert(
	rowRules.rule.every((w) => w === '1px'),
	`row separation is a 1px rule (${rowRules.rule[0]}), not a gap and not a card`,
	`row separation is ${[...new Set(rowRules.rule)].join(' / ')}, expected 1px`
);
assert(
	rowRules.numCellAlign === 'right' && rowRules.numHeaderAlign === 'right',
	`header alignment matches its column's data alignment (both right on a numeric column)`,
	`numeric column: cell ${rowRules.numCellAlign}, header ${rowRules.numHeaderAlign} — a left header ` +
		`over right numbers is a persistent scanning cost`
);
assert(
	rowRules.numCellNumeric.includes('tabular-nums') &&
		!rowRules.proseNumeric.includes('tabular-nums'),
	`tabular-nums is on the numeric cell (${rowRules.numCellNumeric}) and not on prose (${rowRules.proseNumeric})`,
	`tabular-nums: numeric cell "${rowRules.numCellNumeric}", prose cell "${rowRules.proseNumeric}"`
);
assert(
	rowRules.labelTag === 'SPAN' && rowRules.labelHidden === 'true' && rowRules.cellBefore === 'none',
	`the stacked label is a real <span aria-hidden="true">, and no cell carries ::before generated ` +
		`content that would land inside its accessible name`,
	`stacked label: <${rowRules.labelTag}> aria-hidden=${rowRules.labelHidden}, cell ::before=${rowRules.cellBefore}`
);
assert(
	rowRules.monoWrap !== 'anywhere',
	`machine strings do not use overflow-wrap:anywhere (computed "${rowRules.monoWrap}"), which would ` +
		`render x264 as x26 / 4`,
	`overflow-wrap:anywhere on mono content — it breaks at any character and splits x264`
);
assert(
	rowRules.monoTitle !== null && rowRules.monoWhiteSpace === 'nowrap',
	`identity field ellipsises at the cell (white-space:${rowRules.monoWhiteSpace}) with the full string ` +
		`in title (${String(rowRules.monoTitle).slice(0, 40)}…)`,
	`identity field: white-space=${rowRules.monoWhiteSpace}, title=${rowRules.monoTitle}`
);
assert(
	rowRules.maxChips <= 4,
	`chip cells cap at three plus "+N more" (worst rendered cell has ${rowRules.maxChips} chips)`,
	`a chip cell rendered ${rowRules.maxChips} chips, above the three-plus-more cap`
);
assert(
	rowRules.cols !== '' && rowRules.gridTemplate.split(' ').length >= 7,
	`column widths are declared, not derived: --cols resolves to "${rowRules.cols}" and the row's ` +
		`grid-template-columns computes to ${rowRules.gridTemplate.split(' ').length} explicit tracks`,
	`--cols did not resolve ("${rowRules.cols}") — either it was never set or a style ATTRIBUTE was used, ` +
		`which the production CSP refuses`
);
note(`--row-ci resolves to "${rowRules.rowCi}"`);

head('13. The CSSOM route for a custom property, and the style-attribute route');
/* The production CSP is `style-src 'self'` with no 'unsafe-inline', and this
 * harness page carries no CSP at all — so this run only proves the CSSOM route
 * WORKS, not that the attribute route fails. The attribute-under-CSP half is
 * verified against the real binary, whose header is the real one; see the
 * report at the end of the run. */
const cssom = await page.evaluate(() => {
	const tbl = document.querySelector('table.tbl');
	tbl.style.setProperty('--probe', '42px');
	const viaCssom = getComputedStyle(tbl).getPropertyValue('--probe').trim();
	tbl.style.removeProperty('--probe');
	return { viaCssom, hasStyleAttr: tbl.hasAttribute('style') };
});
assert(
	cssom.viaCssom === '42px',
	`element.style.setProperty() sets a custom property that resolves ("${cssom.viaCssom}")`,
	`element.style.setProperty() did not take: "${cssom.viaCssom}"`
);

/* ⚠️ THE TWO `textContent` READS BELOW ARE DELIBERATELY NARROW, AND WIDENING
 * EITHER ONE IS A BUG RATHER THAN A CONVENIENCE.
 *
 * `textContent` reads straight through `display: none` and through `[hidden]`,
 * so a read taken over any tree that holds a hidden VARIANT of what is beside
 * it welds the two into a string no user has ever seen. That is not theoretical
 * here: this harness's own table carries 1,000 such nodes at 200 rows —
 * measured, 800 `.stacklabel` and 200 `.stacksep`, the phone-mode column labels
 * and their separators, which are `display: none` at desktop width and sit
 * INSIDE the `<td>`s. Read a row at 1440px and `Size` fuses onto its own value
 * as "Size1.6 MiB". A sibling checker hit exactly this and manufactured a dozen
 * fabricated strings out of it.
 *
 * ✅ AUDITED, not assumed: both selectors below resolve to a single element with
 * ZERO descendants of any kind (`.empty__title` is `<h2>{emptyTitle}</h2>`) or,
 * for `.banner`, to two nested wrappers with zero `[hidden]`, zero `display:
 * none`, zero `aria-hidden` and no direct text of its own beside its one
 * blockified child — so there is nothing hidden to weld in and nothing to drop.
 * Checked in all five states; `textContent` and a hidden-aware text walk return
 * byte-identical strings in every one. The same walk over the enclosing
 * `.tablewrap` is where the 1,000 appear.
 *
 * So: if you ever need more text than these two strings, walk TEXT NODES and
 * test each node's own ancestry with `closest('[hidden]')` plus a computed
 * `display` check — do not reach for `wrap.textContent`. */
head('14. The states the primitive owns (§10)');
const states = await page.evaluate(async () => {
	const out = {};
	const read = () => {
		const wrap = document.querySelector('.tablewrap');
		const h2 = wrap.querySelector('.empty__title');
		const cs = h2 ? getComputedStyle(h2) : null;
		return {
			hasTable: !!wrap.querySelector('table.tbl'),
			heading: h2?.textContent ?? null,
			headingTag: h2?.tagName ?? null,
			headingSize: cs?.fontSize ?? null,
			headingAlign: cs?.textAlign ?? null,
			// §9.6 forbids a CONTAINER — a dashed border, a box, a panel, a
			// background step. It does not forbid the one rule that draws the top
			// edge the absent table would have had, which is what .empty is: a
			// border-top and nothing else. So the assertion is "three sides bare
			// and no background", not "no border at all".
			container: h2
				? (() => {
						const p = getComputedStyle(h2.parentElement);
						return {
							sides: [p.borderRightStyle, p.borderBottomStyle, p.borderLeftStyle],
							top: p.borderTopStyle,
							background: p.backgroundColor
						};
					})()
				: null,
			banner: wrap.querySelector('.banner')?.textContent?.trim() ?? null
		};
	};
	for (const s of ['default', 'empty', 'filtered-empty', 'partial', 'stale']) {
		window.__harness.harness.setState(s);
		window.__harness.flush();
		out[s] = read();
	}
	window.__harness.harness.setState('default');
	window.__harness.flush();
	return out;
});
assert(
	states.empty.heading !== states['filtered-empty'].heading,
	`filtered-empty is a DIFFERENT message from empty ("${states['filtered-empty'].heading}" vs ` +
		`"${states.empty.heading}")`,
	`empty and filtered-empty render the same message, which blames the wrong thing`
);
const bare =
	states.empty.container.sides.every((s) => s === 'none') &&
	states.empty.container.background === 'rgba(0, 0, 0, 0)';
assert(
	states.empty.headingTag === 'H2' &&
		states.empty.headingSize === '16px' &&
		(states.empty.headingAlign === 'left' || states.empty.headingAlign === 'start') &&
		bare,
	`the empty state is an <h2> at ${states.empty.headingSize} (--text-lg), left-aligned ` +
		`(text-align: ${states.empty.headingAlign}), with no container — three sides bare, no ` +
		`background step, and only the ${states.empty.container.top} top rule where the table's ` +
		`own top edge would be — §9.6`,
	`empty state: <${states.empty.headingTag}> at ${states.empty.headingSize}, ` +
		`text-align ${states.empty.headingAlign}, sides ${states.empty.container.sides.join('/')}, ` +
		`background ${states.empty.container.background}`
);
assert(
	states.partial.banner !== null && states.stale.banner !== null && states.partial.hasTable,
	`partial ("${states.partial.banner}") and stale ("${states.stale.banner}") both render the data ` +
		`alongside the explanation rather than instead of it`,
	`partial/stale did not render both a banner and the table`
);

head('15. The responsive fork below 760 px');
await page.setViewportSize({ width: 390, height: 844 });
await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => r(null))));
const stacked = await page.evaluate(() => {
	const tbl = document.querySelector('table.tbl');
	const thead = tbl.querySelector('thead');
	const row = tbl.querySelector('tbody tr');
	const line1 = row.querySelector('td[data-line="1"]');
	const line2 = [...row.querySelectorAll('td[data-line="2"]')];
	const hidden = [...row.querySelectorAll('td[data-line="hidden"]')];
	const sep = row.querySelector('.stacksep');
	return {
		headerStillInTree: thead.querySelectorAll('[role="columnheader"]').length,
		headerClipped: getComputedStyle(thead).position === 'absolute',
		rowHeight: Math.round(row.getBoundingClientRect().height),
		line1Display: line1 ? getComputedStyle(line1).display : null,
		line2Display: line2[0] ? getComputedStyle(line2[0]).display : null,
		hiddenDisplay: hidden[0] ? getComputedStyle(hidden[0]).display : null,
		sepDisplay: sep ? getComputedStyle(sep).display : null,
		sepAriaHidden: sep ? sep.getAttribute('aria-hidden') : null,
		docScrollWidth: document.documentElement.scrollWidth,
		innerWidth: window.innerWidth
	};
});
assert(
	stacked.headerStillInTree > 0 && stacked.headerClipped,
	`the header row is visually hidden but still ${stacked.headerStillInTree} columnheader nodes in the ` +
		`accessibility tree, so column names survive the stacked view`,
	`the header row is gone from the accessibility tree in the stacked view (${stacked.headerStillInTree} ` +
		`columnheaders, clipped=${stacked.headerClipped})`
);
assert(
	stacked.line1Display === 'block' &&
		stacked.line2Display === 'inline' &&
		stacked.hiddenDisplay === 'none',
	`a results row stacks to TWO lines: ${stacked.rowHeight}px, identity on line 1, two secondary fields ` +
		`inline on line 2, the rest behind the row`,
	`stacked row: line1=${stacked.line1Display}, line2=${stacked.line2Display}, hidden=${stacked.hiddenDisplay}`
);
assert(
	stacked.sepDisplay === 'inline' && stacked.sepAriaHidden === 'true',
	`the "·" between the secondary fields is a real aria-hidden element, never ::before content`,
	`the stacked separator is ${stacked.sepDisplay} / aria-hidden=${stacked.sepAriaHidden}`
);
assert(
	stacked.docScrollWidth <= stacked.innerWidth,
	`nothing pushes the document sideways at 390px (scrollWidth ${stacked.docScrollWidth} <= ${stacked.innerWidth})`,
	`the document scrolls sideways at 390px: ${stacked.docScrollWidth} > ${stacked.innerWidth}`
);

head('16. The sticky header across the 761–1,099 px band');
for (const width of [761, 900, 1099, 1280, 1440]) {
	await page.setViewportSize({ width, height: 900 });
	await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => r(null))));
	const sticky = await page.evaluate(() => {
		const wrap = document.querySelector('.tablewrap');
		const thead = document.querySelector('table.tbl thead');
		window.scrollTo(0, 1200);
		void document.documentElement.offsetHeight;
		const top = thead.getBoundingClientRect().top;
		window.scrollTo(0, 0);
		return {
			top: Math.round(top),
			overflowY: getComputedStyle(wrap).overflowY,
			position: getComputedStyle(thead).position
		};
	});
	assert(
		sticky.position === 'sticky' && sticky.top >= 0 && sticky.top < 200,
		`${String(width).padStart(4)}px: header pinned at y=${sticky.top}, wrapper overflow-y:${sticky.overflowY}`,
		`${String(width).padStart(4)}px: header at y=${sticky.top} after a 1200px scroll (wrapper ` +
			`overflow-y:${sticky.overflowY}) — it scrolled away with the page`
	);
}
await page.setViewportSize({ width: 1440, height: 900 });

/* --- done ----------------------------------------------------------------- */

head('Summary');
if (!ASSERT_ONLY) {
	note(
		`webfont state while measuring: document.fonts.status=${fontState.status}, ` +
			`${fontState.count} face(s)${fontState.faces.length ? ': ' + fontState.faces.join(' | ') : ''} ` +
			`— the harness has no publicDir, so /fonts/*.woff2 404s and these are FALLBACK metrics. ` +
			`Measured identical with the face served and blocked (canvas advance-width probe), because ` +
			`the row line-height is a fixed 18px length rather than a unitless multiplier; the same ` +
			`probe does split the two conditions under a forced line-height:normal, so the null result ` +
			`is a measurement rather than an absence of one.`
	);
	note('contain-intrinsic-size, per density (mean rendered content box):');
	for (const d of ['compact', 'standard', 'relaxed']) {
		note(
			`  ${d.padEnd(9)} one-line ${String(recommendedOneLine[d]).padStart(3)}px ` +
				`(min ${shapes['one-line'][d].min}, max ${shapes['one-line'][d].max}, ` +
				`${shapes['one-line'][d].distinct.length} distinct)   ·   ` +
				`rich ${String(recommended[d]).padStart(3)}px ` +
				`(min ${intrinsic[d].min}, max ${intrinsic[d].max}, ${intrinsic[d].distinct.length} distinct)`
		);
	}
	/* WHICH MECHANISM PRODUCED THE ONE-LINE FIGURES, RE-ESTABLISHED ON EVERY
	 * RUN. The same six numbers come out of two opposite regimes — a floor that
	 * binds, and a floor that is slack over a natural height that happens to
	 * match — so a static sentence here is a claim that can silently flip.
	 * `floorBinding` is measured by dropping `min-height` and seeing whether the
	 * height follows, so this line reports rather than asserts, and the reader
	 * never has to trust the prose above it. The measured scope is
	 * `stack: 'two-line'`, which is what the harness renders. */
	for (const shape of ['one-line', 'rich']) {
		const at = (f) => ['compact', 'standard', 'relaxed'].map((d) => shapes[shape][d][f]).join('/');
		const binding = ['compact', 'standard', 'relaxed'].every((d) => shapes[shape][d].floorBinding);
		const slack = ['compact', 'standard', 'relaxed'].every((d) => !shapes[shape][d].floorBinding);
		note(
			`  ${shape.padEnd(8)} floor ${binding ? 'BINDING' : slack ? 'SLACK' : 'MIXED across densities'}: ` +
				`content box ${at('mode')}, natural ${at('naturalMode')} with min-height:0, ` +
				`against a --row-h of ${at('rowH')}` +
				(slack ? ' — the floor is not what sets these; the content is' : '')
		);
	}
	note('');
	note('density toggle, ms (mean of four changes):');
	note('  rows      cv+list   cv+root  nocv+list  nocv+root');
	for (const r of densityTable) {
		note(
			`  ${String(r.n).padStart(6)}  ${String(r.cv_container).padStart(8)}  ` +
				`${String(r.cv_root).padStart(8)}  ${String(r.nocv_container).padStart(9)}  ` +
				`${String(r.nocv_root).padStart(9)}`
		);
	}
	note('');
	note(
		`DOM-row ceiling at the 100 ms Tier-0 hard fail: ~${Math.round(fitShipped.at100).toLocaleString()} rows ` +
			`on this desktop as shipped, ~${Math.round(fitShipped.at100 / 5).toLocaleString()}–` +
			`${Math.round(fitShipped.at100 / 3).toLocaleString()} on a Pi 5 at ADR-0029's 3–5×.`
	);
}

const rssFinal = await browserRssGb();
if (rssFinal !== null)
	note(
		`browser RSS at the end of the run: ${rssFinal.toFixed(2)} GB — a fresh page per size holds ` +
			`this flat; rebuilding in place reached 8.5 GB by section 4 and died in section 5`
	);

await browser.close();
server.close();
await rm(outDir, { recursive: true, force: true });

/* The ceiling is reported LAST as well as at the point it was hit, because the
 * sections after it print numbers at the sizes that did fit, and a reader
 * skimming the tail would otherwise take a short run for a complete one. */
if (ceilingHit)
	process.stdout.write(
		`\n\x1b[31mCEILING: this machine could not carry ${ceilingHit.n.toLocaleString()} rows ` +
			`(${ceilingHit.what}). Largest size completed: ${largestCompleted.toLocaleString()} rows. ` +
			`Figures at or above ${ceilingHit.n.toLocaleString()} rows were NOT obtained.\x1b[0m\n`
	);

process.stdout.write(
	failures === 0
		? '\n[32mAll assertions passed.[0m\n'
		: `\n[31m${failures} assertion(s) failed.[0m\n`
);
process.exit(failures === 0 ? 0 : 1);
