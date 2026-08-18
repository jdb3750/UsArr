/**
 * Shell geometry probe — the toolbar / sidebar / sticky-offset measurement.
 *
 * WHY IT EXISTS. Asking the toolbar where it ends reports ZERO overlap even
 * while its content paints over the page, because a fixed grid track pins the
 * border box regardless of what overflows it. So nothing here reads
 * `.topbar.getBoundingClientRect().bottom` as the toolbar's bottom. The
 * toolbar's real bottom is the LOWEST PAINTED EDGE of its descendants, and that
 * is what every assertion below compares against.
 *
 * The same principle runs through the rest of it. Every question is put to the
 * browser as the thing a person would actually do, not as the property that is
 * cheapest to read:
 *
 *   · "can the shell be scrolled off the screen?" is a real wheel gesture over
 *     the top bar, not `documentElement.scrollHeight` — programmatic scrolling
 *     works straight through `html { overflow: hidden }`, so a property answers
 *     a different question from the one being asked;
 *   · "does the sticky header pin under the bar?" is measured AFTER scrolling.
 *     At rest it sits where the document put it and proves nothing;
 *   · "does the drawer start where the bar ends?" opens the drawer first, since
 *     below 900px it is not in the box tree until it does;
 *   · "can a keyboard user scroll?" presses the keys.
 *
 * WHAT IT FOUND, so a future reader knows what the numbers are for. `--toolbar-h`
 * was a published bar height that every sticky and fixed offset in the shell
 * read. It was hand-fitted to a two-line bar and a three-line bar broke it: at
 * 200px with a long username the bar's real bottom was 97px while the drawer
 * still started at 68, so the bar painted over the drawer's first 29px and the
 * drawer hung 29px past the bottom of the viewport. The shell is now a
 * viewport-height grid whose second row scrolls, the token is gone, and this
 * probe is the evidence that nothing needed it.
 *
 * WHAT IT FOUND NEXT, which is the price of that shape and is measured in the
 * `keyboard` and `restoration` sections at the foot of this file: moving the
 * scroll off the document also moved it off the browser's default scroller, so
 * the scroll keys and SvelteKit's scroll restoration both stopped working.
 * `$lib/shellscroll` restores both, and the numbers that prove it — including
 * what the platform itself scrolls by, re-measured here rather than remembered
 * — come out of this run.
 *
 * Run: pnpm build && pnpm probe:shell
 */
import { createServer } from 'node:http';
import { createReadStream } from 'node:fs';
import { stat } from 'node:fs/promises';
import { join, extname, resolve as resolvePath } from 'node:path';

const outDir = resolvePath(process.cwd(), 'build');

const MIME = {
	'.html': 'text/html; charset=utf-8',
	'.js': 'text/javascript; charset=utf-8',
	'.css': 'text/css; charset=utf-8',
	'.json': 'application/json; charset=utf-8',
	'.svg': 'image/svg+xml',
	'.woff2': 'font/woff2'
};

const server = createServer(async (req, res) => {
	const path = (req.url ?? '/').split('?')[0];
	let file = join(outDir, path === '/' ? 'index.html' : decodeURIComponent(path));
	if (!file.startsWith(outDir)) return void res.writeHead(403).end();
	try {
		const s = await stat(file);
		if (s.isDirectory()) file = join(file, 'index.html');
		await stat(file);
	} catch {
		file = join(outDir, 'index.html'); // SPA fallback
	}
	res.writeHead(200, { 'content-type': MIME[extname(file)] ?? 'application/octet-stream' });
	createReadStream(file).pipe(res);
});
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const BASE = `http://127.0.0.1:${server.address().port}`;

if (!process.env.PLAYWRIGHT_BROWSERS_PATH)
	process.env.PLAYWRIGHT_BROWSERS_PATH = '/opt/pw-browsers';
const { chromium } = await import('playwright-core');
const browser = await chromium.launch();

const SHORT_USER = 'joe';
const LONG_USER = 'joseph-the-self-hoster-with-a-very-long-account-name';

function session(username) {
	return {
		authenticated: true,
		setup_required: false,
		csrf_token: 'probe',
		user_id: 1,
		username,
		is_owner: true
	};
}

/* Every screen degrades honestly when a service is absent, so an empty upstream
 * is a legitimate render rather than a broken one. The shell geometry this
 * probe measures is the same either way. */
const KINDS = ['prowlarr', 'sonarr', 'radarr', 'lidarr', 'jellyfin', 'navidrome'];
const svc = (i) => ({
	id: i + 1,
	kind: KINDS[i % KINDS.length],
	role: 'indexer',
	name: `${KINDS[i % KINDS.length]}-${i + 1}`,
	base_url: `http://10.0.0.${20 + i}:9696`,
	api_version: 'v1',
	enabled: true,
	priority: i,
	managed_by: 'user',
	verify_tls: true,
	has_credential: true,
	health_state: 'ok',
	breaker_state: 'closed'
});
const health = (i) => ({
	...svc(i),
	state: i % 4 === 0 ? 'error' : 'ok',
	problem: i % 4 === 0 ? 'Connection refused by the upstream host.' : undefined,
	action: i % 4 === 0 ? 'Check the base URL.' : undefined,
	consecutive_failures: i % 4 === 0 ? 3 : 0,
	app_version: '1.2.3',
	warnings: [],
	blocked_indexers: [],
	observed_at: '2026-08-17T12:00:00Z',
	stale: false
});
const ROWS = 24;
const many = (f) => Array.from({ length: ROWS }, (_, i) => f(i));

/* Order matters: /services/health has to be matched before /services. */
const STUBS = [
	[/\/api\/v1\/auth\/session$/, (u) => session(u)],
	[
		/\/api\/v1\/services\/health/,
		() => ({ services: many(health), any_unhealthy: true, setup_required: false })
	],
	[/\/api\/v1\/services/, () => ({ services: many(svc) })],
	[/\/api\/v1\/indexers/, () => ({ indexers: [] })],
	[/\/api\/v1\/grabs\/recent/, () => ({ grabs: [] })],
	[/\/api\/health\/ready/, () => ({ status: 'ok' })]
];

async function newPage(username, width, height) {
	const page = await browser.newPage({ viewport: { width, height } });
	page.on('pageerror', () => {});
	await page.route('**/api/**', async (route) => {
		const url = route.request().url();
		if (url.includes('/api/events')) return route.fulfill({ status: 204, body: '' });
		for (const [re, body] of STUBS) {
			if (re.test(url))
				return route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify(body(username))
				});
		}
		return route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
	});
	return page;
}

/** The whole instrument, run in-page. */
const MEASURE = `(() => {
  const q = (s) => document.querySelector(s);
  const topbar = q('.topbar');
  const main = q('.main');
  const sidebar = q('.sidebar');
  const user = q('.topbar__user');
  const de = document.documentElement;

  /* THE TOOLBAR'S REAL BOTTOM: the lowest painted edge among its descendants,
   * not its own border box. A fixed track pins the border box while the
   * content spills past it, which is exactly the bug this probe exists for. */
  let painted = -Infinity;
  for (const el of topbar.querySelectorAll('*')) {
    const cs = getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden') continue;
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) continue;
    painted = Math.max(painted, r.bottom);
  }
  const tb = topbar.getBoundingClientRect();
  const barBox = tb.bottom;
  const barPainted = painted === -Infinity ? tb.bottom : painted;
  /* The bar's real bottom includes its own border/padding when the box is the
   * taller of the two; when the content spills, the content wins. */
  const barBottom = Math.max(barBox, barPainted);

  /* Flex lines. NOT "distinct top edges": align-items:center gives items of
   * different heights different tops ON THE SAME LINE, which counts four lines
   * in a bar that has one. Items are clustered instead — a new line starts
   * only where an item's top clears the bottom of everything before it. */
  const boxes = [];
  for (const el of topbar.children) {
    const cs = getComputedStyle(el);
    if (cs.display === 'none') continue;
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) continue;
    boxes.push(r);
  }
  boxes.sort((a, b) => a.top - b.top);
  let lineCount = 0;
  let lineBottom = -Infinity;
  for (const r of boxes) {
    if (r.top >= lineBottom - 0.5) { lineCount++; lineBottom = r.bottom; }
    else lineBottom = Math.max(lineBottom, r.bottom);
  }
  const lines = { size: lineCount };

  const mainTop = main.getBoundingClientRect().top;
  const sb = sidebar ? sidebar.getBoundingClientRect() : null;

  /* Any sticky table header currently rendered. */
  const thead = q('.tbl thead');

  return {
    barBox: +barBox.toFixed(1),
    barPainted: +barPainted.toFixed(1),
    barBottom: +barBottom.toFixed(1),
    barSpill: +(barPainted - barBox).toFixed(1),
    lines: lines.size,
    mainTop: +mainTop.toFixed(1),
    /* > 0 means the bar's content paints OVER the page. */
    overlapOverPage: +Math.max(0, barPainted - mainTop).toFixed(1),
    sidebar: sb && {
      top: +sb.top.toFixed(1),
      /* THE REGRESSION: sidebar top minus the bar's real bottom. 0 is right. */
      gapToBar: +(sb.top - barBottom).toFixed(1),
      height: +sb.height.toFixed(1),
      remaining: +(innerHeight - barBottom).toFixed(1),
      overshoot: +(sb.height - (innerHeight - barBottom)).toFixed(1),
      clipped: sidebar.scrollHeight - sidebar.clientHeight,
      scrollable: getComputedStyle(sidebar).overflowY
    },
    /* Below 900px the header is VISUALLY HIDDEN (.tbl--stack / .tbl--2line put
     * it at position:absolute behind a clip-path) so its rect says nothing
     * about any sticky offset. Flagged rather than dropped, so a reading of
     * "not sticky here" is a fact about the design and not a hole in the probe. */
    thead: thead && {
      top: +thead.getBoundingClientRect().top.toFixed(1),
      stickyTop: getComputedStyle(thead).top,
      visuallyHidden: getComputedStyle(thead).position === 'absolute'
    },
    user: user && {
      text: user.textContent.trim().length,
      clientW: user.clientWidth,
      scrollW: user.scrollWidth,
      truncated: user.scrollWidth > user.clientWidth,
      ellipsis: getComputedStyle(user).textOverflow,
      whiteSpace: getComputedStyle(user).whiteSpace,
      tooltip: user.getAttribute('title') || null
    },
    /* The per-page toolbar, whose min-height went with the token. */
    pageToolbars: [...document.querySelectorAll('.toolbar')].map((t) =>
      +t.getBoundingClientRect().height.toFixed(1)
    ),
    /* Whichever box is the scroll container has to be the one that scrolls. */
    mainScrollable: main.scrollHeight - main.clientHeight,
    docScrollH: de.scrollHeight,
    docClientH: de.clientHeight,
    hScroll: de.scrollWidth - de.clientWidth
  };
})()`;

async function measure(page, route, width, height, { openDrawer } = {}) {
	await page.goto(BASE + route);
	await page.waitForSelector('.topbar__user, .topbar', { timeout: 10000 });
	await page.waitForFunction('document.fonts.ready.then(() => true)');
	// A drawer at <=900px: the sidebar does not exist in the box tree until it opens.
	if (openDrawer) {
		const toggle = page.locator('.sidebar-toggle');
		if (await toggle.count()) {
			await toggle.click();
			await page.waitForSelector('.sidebar', { state: 'visible' });
		}
	}
	await page.evaluate(
		() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
	);
	const at0 = await page.evaluate(MEASURE);

	/* ⚠️ A REAL WHEEL OVER THE TOP BAR, where no inner scroller can consume it,
	 * so the question "can the user scroll the shell itself off the screen?" is
	 * answered by a gesture rather than by a property. `documentElement.
	 * scrollHeight` alone is not the answer and `html { overflow: hidden }` is
	 * not either — programmatic scrolling still works through both. */
	await page.mouse.move(Math.floor(width / 2), 8);
	await page.mouse.wheel(0, 600);
	await page.waitForTimeout(200);
	const wheel = await page.evaluate(() => ({
		y: window.scrollY,
		barTop: +document.querySelector('.topbar').getBoundingClientRect().top.toFixed(1),
		docSH: document.documentElement.scrollHeight,
		docCH: document.documentElement.clientHeight
	}));
	await page.evaluate(() => window.scrollTo(0, 0));

	/* THE STICKY TABLE HEADER, measured PINNED rather than at rest. At rest it
	 * sits wherever the document put it and says nothing about its offset.
	 * Scrolls whichever box is the scroll container, so the same probe reads a
	 * document-scrolled shell and a main-scrolled one. */
	await page.evaluate(() => {
		window.scrollBy(0, 600);
		const m = document.querySelector('.main');
		if (m) m.scrollTop = 600;
	});
	await page.evaluate(
		() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
	);
	const scrolled = await page.evaluate(MEASURE);
	return {
		...at0,
		wheel,
		pinned: scrolled.thead ? { ...scrolled.thead, barBottom: scrolled.barBottom } : null
	};
}

const ROUTES = ['/', '/services', '/search', '/libraries', '/settings'];
const WIDE = [1280, 1024, 940];
const NARROW = [560, 480, 390, 320, 260, 200];

const out = [];
for (const [label, username] of [
	['short', SHORT_USER],
	['long', LONG_USER]
]) {
	for (const w of [...WIDE, ...NARROW]) {
		const narrow = w <= 900;
		const page = await newPage(username, w, 800);
		for (const route of ROUTES) {
			// Narrow widths only get the full route sweep at 560 and 320; the rest
			// is a width sweep on Home, which is where the bar is busiest.
			if (narrow && ![560, 320].includes(w) && route !== '/') continue;
			if (!narrow && ![1280].includes(w) && route !== '/') continue;
			const m = await measure(page, route, w, 800, { openDrawer: narrow });
			out.push({ nameLen: label, w, route, ...m });
		}
		await page.close();
	}
}

/* ── KEYBOARD SCROLLING, WHICH IS THE COST OF THE SHAPE ──────────────────────
 *
 * Moving the scroll from the document to `.main` moves it off the browser's
 * default scroller, and the browser hands a key press to the nearest scrollable
 * ancestor OF THE FOCUSED ELEMENT. `<body>` no longer has one, so on the very
 * first paint — which is the only moment focus sits on the body, since
 * `afterNavigate` focuses the h1 inside main on every navigation after arrival
 * — PageDown scrolled nothing at all.
 *
 * `$lib/shellscroll` supplies the missing ancestor: while focus is still on the
 * body it forwards the scroll keys to `.main`. Everything below is the evidence
 * that it does that and NOTHING else — the amounts match what the platform
 * itself does, focus never moves, and the handler is inert the moment focus is
 * anywhere real.
 *
 * Measured rather than reasoned about, and printed rather than remembered. */

/**
 * The keys a browser actually scrolls with, each with a starting offset that
 * leaves room to move in the direction under test. `axis: 'x'` cases are read
 * on scrollLeft instead of scrollTop.
 */
const KEY_CASES = [
	{ name: 'ArrowDown', press: 'ArrowDown', from: 0 },
	{ name: 'ArrowUp', press: 'ArrowUp', from: 600 },
	{ name: 'PageDown', press: 'PageDown', from: 0 },
	{ name: 'PageUp', press: 'PageUp', from: 1200 },
	{ name: 'Space', press: 'Space', from: 0 },
	{ name: 'Shift+Space', press: 'Shift+Space', from: 1200 },
	{ name: 'End', press: 'End', from: 0 },
	{ name: 'Home', press: 'Home', from: 1200 },
	{ name: 'ArrowRight', press: 'ArrowRight', from: 0, axis: 'x' },
	{ name: 'ArrowLeft', press: 'ArrowLeft', from: 600, axis: 'x' }
];

/* Combinations the handler must NOT touch. Reported as `prevented`, which is
 * the only thing that matters here: swallowing one of these is the failure. */
const MODIFIED_CASES = [
	'Control+PageDown',
	'Control+ArrowDown',
	'Control+End',
	'Control+Home',
	'Alt+ArrowDown',
	'Meta+ArrowDown',
	'Shift+ArrowDown',
	'Control+Space'
];

/* Registered AFTER the shell's own window listener, so it runs second in the
 * same bubble phase and reads whatever that listener decided. `prevented` is
 * therefore a direct measurement of "did the shell swallow this key", not an
 * inference from the scroll offset. */
const INSTRUMENT = `(() => {
  window.__probeKeys = [];
  window.addEventListener('keydown', (e) => {
    window.__probeKeys.push({ key: e.key, prevented: e.defaultPrevented });
  });
})()`;

const describeActive = `(() => {
  const a = document.activeElement;
  if (!a) return null;
  const cls = typeof a.className === 'string' && a.className ? '.' + a.className.trim().split(/\\s+/).join('.') : '';
  return a.tagName + cls;
})()`;

const readState = `(() => {
  const main = document.querySelector('.main');
  const sidebar = document.querySelector('.sidebar');
  return {
    focus: ${describeActive},
    focusIsBody: document.activeElement === document.body,
    top: main.scrollTop,
    left: main.scrollLeft,
    maxTop: main.scrollHeight - main.clientHeight,
    clientH: main.clientHeight,
    sidebarTop: sidebar ? sidebar.scrollTop : null,
    windowY: window.scrollY,
    keys: (window.__probeKeys || []).splice(0)
  };
})()`;

const twoFrames = () => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));

/* ⚠️ CHROMIUM ANIMATES KEYBOARD SCROLLING, in the compositor and independently
 * of `scroll-behavior`. Reading an offset a fixed 150ms after the key press
 * therefore samples the animation MID-FLIGHT: the first run of this probe read
 * PageDown as 679px and Home as "1664 → 10", neither of which is a scroll
 * amount. Every offset below is read only once it has stopped moving. */
async function settled(page, expr) {
	let prev = null;
	for (let i = 0; i < 30; i++) {
		const v = await page.evaluate(expr);
		if (prev !== null && v === prev) return v;
		prev = v;
		await page.waitForTimeout(70);
	}
	return prev;
}

async function settle(page, selector) {
	await page.waitForSelector(selector, { timeout: 10000 });
	await page.waitForFunction('document.fonts.ready.then(() => true)');
	await page.evaluate(twoFrames);
}

/** Presses one key from a known offset and reports what moved. */
async function pressFrom(page, c) {
	await page.evaluate(
		(v) => {
			const m = document.querySelector('.main');
			m.scrollTop = v.y;
			m.scrollLeft = v.x;
			if (window.__probeKeys) window.__probeKeys.length = 0;
		},
		c.axis === 'x' ? { x: c.from, y: 0 } : { x: 0, y: c.from }
	);
	const axisExpr =
		c.axis === 'x'
			? 'document.querySelector(".main").scrollLeft'
			: 'document.querySelector(".main").scrollTop';
	await settled(page, axisExpr);
	const before = await page.evaluate(readState);
	await page.keyboard.press(c.press);
	await page.waitForTimeout(80);
	await settled(page, axisExpr);
	const after = await page.evaluate(readState);
	return {
		axis: c.axis ?? 'y',
		from: c.axis === 'x' ? before.left : before.top,
		to: c.axis === 'x' ? after.left : after.top,
		delta: c.axis === 'x' ? after.left - before.left : after.top - before.top,
		focusBefore: before.focus,
		focusAfter: after.focus,
		windowY: after.windowY,
		keys: after.keys
	};
}

/* ── WHAT THE PLATFORM ITSELF DOES ───────────────────────────────────────────
 * The amounts are not assumed. A plain document that DOES scroll, in the same
 * Chromium, at the same viewport, is asked what each key is worth; the shell's
 * forwarding is then compared against these numbers rather than against a
 * remembered constant. */
const platform = await (async () => {
	const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
	/* ⚠️ THE DOCTYPE IS LOAD-BEARING. Without it setContent renders in QUIRKS
	 * mode, where `documentElement.clientHeight` is the element's own height
	 * rather than the viewport's — the first run of this probe reported a 20000px
	 * "viewport" and a maxY of 0 for exactly that reason. */
	await page.setContent(
		'<!doctype html><style>html,body{margin:0;padding:0}#tall{width:6000px;height:20000px}</style><div id="tall"></div>'
	);
	await page.evaluate(twoFrames);
	const rows = {};
	for (const c of KEY_CASES) {
		await page.evaluate(
			(v) => window.scrollTo(v.x, v.y),
			c.axis === 'x' ? { x: c.from, y: 0 } : { x: 0, y: c.from }
		);
		await settled(page, c.axis === 'x' ? 'scrollX' : 'scrollY');
		const b = await page.evaluate(() => ({ x: scrollX, y: scrollY }));
		await page.keyboard.press(c.press);
		await page.waitForTimeout(80);
		await settled(page, c.axis === 'x' ? 'scrollX' : 'scrollY');
		const a = await page.evaluate(() => ({ x: scrollX, y: scrollY }));
		rows[c.name] = {
			axis: c.axis ?? 'y',
			from: c.axis === 'x' ? b.x : b.y,
			to: c.axis === 'x' ? a.x : a.y,
			delta: c.axis === 'x' ? a.x - b.x : a.y - b.y
		};
	}
	const box = await page.evaluate(() => ({
		clientH: document.documentElement.clientHeight,
		clientW: document.documentElement.clientWidth,
		maxY: document.documentElement.scrollHeight - document.documentElement.clientHeight
	}));
	await page.close();
	return {
		...box,
		/* Blink's page step: max(h * 0.875, h - 40). Printed alongside the
		 * measurement so the two can be compared rather than trusted. */
		blinkPageStep: Math.max(box.clientH * 0.875, box.clientH - 40),
		keys: rows
	};
})();

const kb = await (async () => {
	const steps = { platform };

	/* THE HEADLINE: a page that has just painted, focus untouched, one key. */
	{
		const page = await newPage(SHORT_USER, 1280, 800);
		await page.goto(BASE + '/services');
		await settle(page, '.tbl tbody tr');
		await page.evaluate(INSTRUMENT);
		const before = await page.evaluate(readState);
		await page.keyboard.press('PageDown');
		await page.waitForTimeout(200);
		const after = await page.evaluate(readState);
		steps.firstPaintPageDown = { route: '/services', viewport: '1280x800', before, after };
		await page.close();
	}

	/* THE SWEEP, on one page that is never clicked, so focus stays on <body>
	 * for the whole run and every row below is a body-focus measurement. */
	{
		const page = await newPage(SHORT_USER, 1280, 800);
		await page.goto(BASE + '/services');
		await settle(page, '.tbl tbody tr');
		await page.evaluate(INSTRUMENT);
		steps.sweepFromBody = { route: '/services', viewport: '1280x800', keys: {} };
		for (const c of KEY_CASES) steps.sweepFromBody.keys[c.name] = await pressFrom(page, c);
		steps.sweepFromBody.modified = {};
		for (const combo of MODIFIED_CASES) {
			steps.sweepFromBody.modified[combo] = await pressFrom(page, {
				name: combo,
				press: combo,
				from: 600
			});
		}
		steps.sweepFromBody.focusAfterEverything = await page.evaluate(readState);

		/* TAB ORDER, on the same untouched page: the skip link is the promise. */
		const stops = [];
		for (let i = 0; i < 3; i++) {
			await page.keyboard.press('Tab');
			await page.waitForTimeout(80);
			stops.push(
				await page.evaluate(
					`(() => ({ el: ${describeActive}, text: (document.activeElement.textContent || '').trim().slice(0, 40) }))()`
				)
			);
		}
		steps.tabOrder = stops;
		await page.close();
	}

	/* FOCUS SOMEWHERE REAL. Three shapes of "real": a link that lives inside a
	 * scrollable region of its own, a button, and a text field. In all three the
	 * handler must be inert — no scroll and, more importantly, no preventDefault,
	 * since swallowing a key something else would have handled is the worse bug. */
	{
		const page = await newPage(SHORT_USER, 1280, 800);
		await page.goto(BASE + '/services');
		await settle(page, '.tbl tbody tr');
		await page.evaluate(INSTRUMENT);
		const real = {};

		await page.focus('.sidebar .nav__link');
		real.navLink = { focus: await page.evaluate(describeActive), keys: {} };
		for (const c of KEY_CASES) real.navLink.keys[c.name] = await pressFrom(page, c);

		const addBtn = page.locator('button', { hasText: 'Add service' }).first();
		if (await addBtn.count()) {
			await addBtn.focus();
			real.button = { focus: await page.evaluate(describeActive), keys: {} };
			// Space and Enter would ACTIVATE the button, so the scroll keys that do
			// not activate anything are the ones pressed here.
			for (const c of KEY_CASES.filter((c) => !c.press.includes('Space'))) {
				real.button.keys[c.name] = await pressFrom(page, c);
			}
		}

		/* THE DOCUMENTED FIRST TAB STOP. Once the skip link has moved focus into
		 * the region the browser is scrolling it again on its own, which is the
		 * whole point of the forwarding being narrow: it stops mattering here. */
		await page.evaluate(() => {
			document.querySelector('.main').scrollTop = 0;
			document.querySelector('.skip').click();
		});
		await page.waitForTimeout(120);
		real.afterSkipLink = await pressFrom(page, { name: 'PageDown', press: 'PageDown', from: 0 });
		real.afterSkipLink.focus = await page.evaluate(describeActive);

		steps.focusReal = real;
		await page.close();
	}

	/* A TEXT FIELD, at a viewport short enough that the region really can scroll,
	 * so "did not scroll" is a fact rather than an artefact of a short page. */
	{
		const page = await newPage(SHORT_USER, 1280, 400);
		await page.goto(BASE + '/requests');
		await settle(page, '#req-query');
		await page.evaluate(INSTRUMENT);
		const scrollable = await page.evaluate(
			() =>
				document.querySelector('.main').scrollHeight - document.querySelector('.main').clientHeight
		);
		await page.focus('#req-query');
		await page.keyboard.type('abc');
		const keys = {};
		for (const c of KEY_CASES) keys[c.name] = await pressFrom(page, c);
		steps.textField = {
			route: '/requests',
			viewport: '1280x400',
			regionScrollable: scrollable,
			focus: await page.evaluate(describeActive),
			value: await page.inputValue('#req-query'),
			keys
		};
		await page.close();
	}

	return steps;
})();

/* ── SCROLL RESTORATION ──────────────────────────────────────────────────────
 *
 * SvelteKit remembers a position per history entry and restores it by calling
 * `scrollTo` on the WINDOW, which no longer moves — so before the shell kept
 * its own record, every back-navigation landed silently at the top. Measured as
 * a user would do it: scroll, navigate, go back, go forward. */
const restoration = await (async () => {
	const page = await newPage(SHORT_USER, 1280, 400);
	const top = () => page.evaluate(() => document.querySelector('.main').scrollTop);
	const setTop = (v) =>
		page.evaluate((v) => {
			document.querySelector('.main').scrollTop = v;
		}, v);

	await page.goto(BASE + '/services');
	await settle(page, '.tbl tbody tr');
	await setTop(500);
	const servicesAt = await top();

	await page.locator('.sidebar .nav__link[href$="/requests"]').click();
	await settle(page, '#req-query');
	const requestsOnArrival = await top();
	await setTop(200);
	const requestsAt = await top();

	await page.goBack();
	await settle(page, '.tbl tbody tr');
	await page.waitForTimeout(500);
	const backToServices = await top();

	await page.goForward();
	await settle(page, '#req-query');
	await page.waitForTimeout(500);
	const forwardToRequests = await top();

	await page.close();
	return {
		viewport: '1280x400',
		servicesAt,
		/* A NEW navigation belongs at the top of the new screen, and the region
		 * carries its own scroll across a route change unless something resets it. */
		requestsOnArrival,
		requestsAt,
		backToServices,
		forwardToRequests
	};
})();

console.log(JSON.stringify({ geometry: out, keyboard: kb, restoration }, null, 1));
await browser.close();
server.close();
