/* =============================================================================
 * UsArr — the design check. One entry point, one exit code.
 *
 *   PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers node docs/design/check.mjs
 *
 * This is the enforcement mechanism for DESIGN-DIRECTION.md §13. Everything it
 * asserts was, at some point, a defect a human found by reading — and every one
 * of them is cheaper to find here. It replaces `docs/design/mockups/selftest.mjs`
 * (folded in whole) and the §13 ban sweep, which up to now was retyped by hand
 * each review round and rediscovered the same four false positives every time.
 *
 * WHAT INVOKES IT: nothing. A person or an agent types `make design`, and that
 * is the whole of it — there is no CI in this repo and never has been
 * (REVIEW-LOG NOCI-01, which established the absence across the tracked tree,
 * every remote ref and the Actions API), and no document requires this target
 * the way CLAUDE.md requires `make check` (REVIEW-LOG OPTIN-01). Comments here
 * used to name a CI as a place this script runs and a place its measurements
 * are taken from; both are corrected, because a guard that misdescribes its own
 * invocation is OPTIN-01 stated in the first person. State what runs it, not
 * what one imagines runs it.
 *
 * TWO DESIGN RULES FOR THIS FILE, and they are the reason it exists rather than
 * a shell pipeline:
 *
 *   1. IT PRINTS WHAT IT CHECKED, NOT ONLY WHAT FAILED. A silent pass is
 *      indistinguishable from a check that matched nothing because its glob was
 *      wrong. Every check prints a count of what it looked at.
 *
 *   2. FALSE POSITIVES ARE EXCLUDED STRUCTURALLY, NEVER BY NAME. The hand-run
 *      sweep kept rediscovering four of them, and each round re-explained them
 *      in prose that was then thrown away. They are gone here because of where
 *      each ban is evaluated, not because a list names them:
 *
 *        · `ArrowRight` as a KeyboardEvent.key. §13's rule is "no Sparkles,
 *          Zap, Shield, BarChart3 or ArrowRight *imports*". So the icon bans
 *          are evaluated over IMPORT SPECIFIERS and over the mockup's icon
 *          vocabulary (`<symbol id="i-…">`), and nowhere else. `ev.key ===
 *          'ArrowRight'` is neither, so it never enters. See banIcons().
 *
 *        · `Inter` inside "Internally" and "The Zone of Interest". §13's rule
 *          is about FONT STACKS. So the font bans are evaluated over the
 *          comma-separated family list of `font-family` and `--font-*`
 *          declarations, matched as WHOLE family names. Prose containing the
 *          substring is not a font stack and never enters. See banFonts().
 *
 *        · `BOOM! Studios`, a real publisher, against the "no `!` in UI
 *          strings" rule. The copy bans are evaluated over RENDERED TEXT NODES
 *          THAT ARE NOT INSIDE A `<td>`. A `<td>` holds data, not the
 *          product's own voice — and that is not an assumption, it is an
 *          invariant this same file enforces two checks further down: the row
 *          height band is what keeps prose out of cells. Data is exempt from
 *          copy rules because data is not copy. See banCopy().
 *
 *        · A comment quoting the ban it documents. Every source-level scan
 *          runs over the file with comments STRIPPED. A rule cannot fire on
 *          its own documentation. See strip().
 *
 * WHAT IT COVERS
 *   1  §13 ban list                     — static, over stripped sources
 *   2  token drift                      — tokens.css vs the mockup's copy
 *   3  contrast                         — worst of all five grounds, both themes
 *   1c install invariant               — data-when and data-inst never collide
 *   4  overflow                         — 5 widths x 5 screens x every state x 2 installs x every panel
 *   5  row heights                      — against the three density bands
 *   6  availability accessible names    — no silent ✓/✗
 *   7  one tab stop per list            — roving tabindex, or a declared opt-out
 *   8  containment is live              — the content-visibility-on-<tr> guard
 *   8b install switcher                 — two installs, and every screen answers
 *   8c panel switchers                  — every panel reachable, defaults asserted
 *   9  webfont                          — IBM Plex actually resolves
 *
 * EVERY RENDERED SWEEP RUNS OVER BOTH INSTALLS, AND OVER EVERY PANEL. Three
 * screens hide content behind a `[data-group][data-panel]` switcher, nothing
 * here had ever clicked one, and the two poster grids therefore sat outside
 * every sweep in this file for its whole existence. See PANEL TRAVERSAL below. The mockups draw a full stack
 * (six populated media types) and the v0.1 install (Sonarr, Radarr, Prowlarr),
 * and half the layouts this file protects only exist on one of the two.
 *
 * THREE THINGS ANY GEOMETRY MEASUREMENT OVER THESE PAGES GETS WRONG FIRST TIME.
 * All three were found the expensive way while verifying a change against a
 * before/after render comparison, and every one of them produces a difference
 * that reads exactly like a layout regression. They are written here rather
 * than in a scratch file because the next person to add a measurement to this
 * file will hit them in the same order.
 *
 *   1. THE WEBFONT IS A RACE. The five screen files LINK fonts.css, so text
 *      metrics change when it lands. A fixed settle after goto() sometimes
 *      measures pre-font metrics and sometimes post-, and the flip is not
 *      deterministic across runs because it depends on how big the other files
 *      are. Symptom: sub-pixel row-height drift concentrated in one long list,
 *      with fractional y-offsets accumulating down the page. Fix:
 *      `await page.evaluate(() => document.fonts.ready)` before measuring.
 *
 *   2. content-visibility: auto MAKES OFF-SCREEN ROWS REPORT A PLACEHOLDER.
 *      A skipped row's getBoundingClientRect() returns its
 *      contain-intrinsic-size, not its laid-out height, and WHICH rows are
 *      skipped depends on render history rather than on layout. Symptom:
 *      heights that look like real numbers with a suspiciously constant
 *      fractional part (47.06, 3567.05, 64.05), differing between two trees
 *      that lay out identically. Fix: force `content-visibility: visible` for
 *      the duration of the measurement and compare the placeholder separately
 *      as a computed value. This one is the dangerous one: it looks like a
 *      regression in exactly the lists a list change would have touched.
 *
 *   3. getBoundingClientRect IS VIEWPORT-RELATIVE. Anything that scrolls the
 *      page between two measurements -- a panel click, a hash assignment, a
 *      focus() -- shifts every rect on the page uniformly. Symptom: a constant
 *      offset (here, the 40px sticky topbar) applied to element 0 onward. Fix:
 *      scrollTo(0, 0) before measuring.
 *
 * AND ONE THING THE MOCKUP STYLESHEET DOES ON PURPOSE, because it looks like a
 * typo: every selector in usarr.css §2.13 names its class twice
 * (`.u-mt-6.u-mt-6`). Those classes replaced inline style attributes, an inline
 * declaration outranks every rule in the sheet, and a single-class replacement
 * therefore loses to rules the inline used to beat -- `.tbl td > .stacklabel +
 * *` at (0,2,0) and a media-query `.select` cap among them. Doubling restores
 * exactly the precedence the attribute had. The reasoning is in §2.13; do not
 * "fix" it here.
 * ============================================================================= */

import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { dirname, join, relative } from 'node:path';
import { checkProvenance, mockupSources } from './provenance.mjs';

const DESIGN = dirname(fileURLToPath(import.meta.url));
const MOCKUPS = join(DESIGN, 'mockups');
const ROOT = join(DESIGN, '..', '..');
const URL = 'file://' + join(MOCKUPS, 'prototype.html');

/* ---------------------------------------------------------------------------
 * ONE LINE, PRINTED FIRST: WHICH TREE THIS RUN ACTUALLY CHECKED.
 *
 * A green that does not name its tree is a rumour. Every other line below
 * reports a count, per this file's rule 1, and none of them says WHICH
 * revision of the specification and the mockups those counts were taken over.
 * That gap is not theoretical: a working tree that predates a specification
 * change passes silently, and the pass is then quoted at a later tree it was
 * never run on. This line makes a stale green self-identifying.
 *
 * THE DIRTY FLAG IS THE LOAD-BEARING HALF. The commit is resolved with no
 * assumption that the tree is clean, because it usually is not: this check is
 * run mid-edit. On a dirty tree the SHA names the last commit and NOT what was
 * checked, so printing the SHA alone would be a more confident lie than
 * printing nothing. Say which of the two the reader is holding.
 *
 * It never fails the run. Absent git, or a directory that is not a repository,
 * is a fact about provenance and not a design defect, so it is reported in the
 * same line rather than raised as a failure this file has no standing to make.
 * ------------------------------------------------------------------------- */
console.log('HEAD  ' + (() => {
  let git;
  try { git = createRequire(import.meta.url)('node:child_process'); } catch { git = null; }
  if (!git) return 'no child_process: this run names no commit, so its result is evidence about no particular tree';
  const run = (...args) => git.execFileSync('git', args, { cwd: ROOT, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
  let sha;
  try { sha = run('rev-parse', 'HEAD').trim(); } catch {
    return 'git unavailable or not a repository: this run names no commit, so its result is evidence about no particular tree';
  }
  /* Resolved separately, and a failure here is not fatal: a repository can
     answer rev-parse and still refuse status (a broken index, a lock held by
     another process). Unknown is reported as unknown rather than as clean. */
  let dirty;
  try { dirty = run('status', '--porcelain').trim().length > 0; } catch { dirty = null; }
  if (dirty === null) return sha + ', working tree UNKNOWN: git could not report status, so it is not known whether this SHA describes what was checked';
  if (dirty) return sha + ', working tree DIRTY: the SHA names the last commit, NOT what this run checked';
  return sha + ', working tree clean: this run checked exactly that commit';
})());

/* ---------------------------------------------------------------------------
 * Playwright is resolved rather than hard-coded, and the failure is explained.
 *
 * This import used to be an ABSOLUTE path into one container's global npm root
 * (/opt/node22/lib/node_modules/playwright/index.mjs). That resolved nowhere
 * else, so `make design` died with a raw ERR_MODULE_NOT_FOUND on the owner's
 * machine and on any other, past the Makefile guard whose whole job is to say
 * something friendlier — and an out-of-tree import cannot be pinned, which the
 * Makefile's "@latest is FORBIDDEN, pin everything" rule forbids on its own.
 *
 * The primary path is now the BARE specifier, which is what a pinned
 * devDependency gives you. Fallbacks exist because this file lives in
 * docs/design/ and the dependency belongs in web/package.json: ESM resolves a
 * bare specifier by walking up from the IMPORTING file, which never reaches
 * web/node_modules, so that location is probed explicitly. The npm global root
 * is probed last, which is what this repo's agent container has.
 *
 * EVERY LOCATION IS PROBED FOR BOTH PACKAGE NAMES. `web/package.json` pins
 * `playwright-core`, not `playwright`, and a ladder that only ever asked for
 * `playwright` walked straight past it — the pin was decorative, and the check
 * ran on whatever the machine happened to have globally, which is the opposite
 * of what pinning is for. `playwright-core` is the right dependency here: it
 * exports the same `chromium` and omits the test runner and the browser
 * downloader this file has no use for. It also does not fetch a browser on
 * install, which is why the two failure modes below are genuinely distinct.
 *
 * TWO FAILURE MODES, TWO MESSAGES. They have nothing in common but the word
 * "playwright", and one message covering both sent people to the expensive fix
 * for the cheap problem:
 *
 *   · THE MODULE IS MISSING. Catchable only with a dynamic `await import()`
 *     inside try/catch — a static `import` is hoisted and throws before any
 *     handler in this file exists, so the user gets a raw
 *     ERR_MODULE_NOT_FOUND. The fix is `pnpm -C web install`.
 *   · THE BROWSER IS ABSENT OR MISMATCHED. Thrown later, from
 *     `chromium.launch()`, and identified by "Executable doesn't exist at".
 *     The likely cause is named first and it is NOT "you never downloaded it":
 *     it is that the module version and the browser cache disagree, because
 *     each Playwright release pins its own browser revision and a cache filled
 *     by a different release satisfies neither. `npx playwright install` does
 *     make the symptom go away, expensively, by fetching a second browser
 *     build — and it hides the version skew that caused it, so it is mentioned
 *     last and with that caveat attached.
 * ------------------------------------------------------------------------- */

/* Kept beside the messages it is printed in, so the two cannot drift. This is
   the version the check is known to pass against; it is what web/package.json
   should pin, exactly, with no caret. */
const PLAYWRIGHT_VERSION = '1.56.1';

/* Both names, in this order. `playwright` is a superset, so if a machine has
   it there is no reason to prefer the core package; if it does not, the pin
   in web/package.json is found on the very next attempt. */
const PW_PACKAGES = ['playwright', 'playwright-core'];

async function loadChromium() {
  const req = createRequire(import.meta.url);
  const nodeRoot = join(dirname(process.execPath), '..', 'lib', 'node_modules');
  const attempts = [];

  /* Each location is expanded across both package names below, so adding a
     third name here needs no new rung. */
  const locations = [
    // An explicit override, for a machine that keeps Playwright somewhere else.
    // This one is a full module path, so it is not expanded.
    process.env.PLAYWRIGHT_MODULE &&
      { how: '$PLAYWRIGHT_MODULE', spec: process.env.PLAYWRIGHT_MODULE, exact: true },
    // The portable path: a pinned devDependency resolvable from this file.
    { how: 'bare specifier', bare: true },
    // web/package.json is where the pinned devDependency lives; ESM will not
    // walk into it from docs/design/, so resolve it the way web/ itself would.
    { how: 'web/node_modules', resolveFrom: join(ROOT, 'web', 'package.json') },
    // The npm global root, which is what the agent container preinstalls.
    { how: 'npm global root', root: nodeRoot },
  ].filter(Boolean);

  const candidates = [];
  for (const loc of locations) {
    if (loc.exact) { candidates.push(loc); continue; }
    for (const pkg of PW_PACKAGES) {
      candidates.push({
        how: `${loc.how} '${pkg}'`,
        pkg,
        spec: loc.bare ? pkg : (loc.root ? join(loc.root, pkg) : undefined),
        resolveFrom: loc.resolveFrom,
      });
    }
  }

  for (const c of candidates) {
    try {
      let spec = c.spec;
      if (c.resolveFrom) spec = createRequire(c.resolveFrom).resolve(c.pkg);
      // A filesystem path has to become a URL before import() will take it.
      const url = /^[./]|^[A-Za-z]:\\/.test(spec) ? pathToFileURL(req.resolve(spec)).href : spec;
      // Dynamic, and inside the try, on purpose: this is the ONLY form that
      // turns a missing module into a message instead of an uncaught throw.
      const mod = await import(url);
      // A CJS entry point arrives as a default-wrapped namespace, so both
      // shapes are accepted rather than only the ESM one.
      const chromium = mod?.chromium || mod?.default?.chromium;
      if (chromium) {
        console.log(`      playwright resolved via ${c.how}`);
        return chromium;
      }
      attempts.push(c.how + ': loaded but exports no chromium');
    } catch (err) {
      attempts.push(c.how + ': ' + (err?.code || err?.message || String(err)));
    }
  }

  console.log('FAIL  the playwright module is not installed, so the design check cannot run.');
  console.log('');
  console.log('      This is the MODULE, not the browser. Nothing was launched and no');
  console.log('      browser cache was consulted; the import itself found nothing.');
  console.log('');
  console.log('      docs/design/check.mjs drives a real Chromium; there is no static fallback.');
  console.log('      web/package.json already pins the dependency, so the fix is usually just:');
  console.log('');
  console.log('          pnpm -C web install');
  console.log('');
  console.log('      If the pin is genuinely absent, add it exactly, with no caret:');
  console.log('');
  console.log('          pnpm -C web add -D --save-exact playwright-core@' + PLAYWRIGHT_VERSION);
  console.log('');
  console.log('      Then re-run `make design`. If Playwright lives somewhere else on this');
  console.log('      machine, point PLAYWRIGHT_MODULE at its module entry point.');
  console.log('');
  console.log('      Resolution attempts, in order:');
  for (const a of attempts) console.log('        - ' + a);
  process.exit(1);
}

const chromium = await loadChromium();

/* Launch, and separate a missing/mismatched browser from every other launch
 * failure. Also records which binary was actually spawned: Playwright picks
 * the headless shell for a plain `launch()` but `executablePath()` keeps
 * reporting the full build, so the only honest way to know what ran is to
 * watch the spawn. The child_process module object is patched rather than the
 * ESM import, because Playwright is CJS internally and `require`s it. */
async function launchChromium() {
  const cp = createRequire(import.meta.url)('node:child_process');
  const origSpawn = cp.spawn;
  const spawned = [];
  cp.spawn = function (cmd, ...rest) { spawned.push(String(cmd)); return origSpawn.call(this, cmd, ...rest); };
  try {
    const browser = await chromium.launch();
    return { browser, spawned };
  } catch (err) {
    const msg = String(err?.message || err);
    if (!/Executable doesn't exist at/.test(msg)) throw err;
    const cache = process.env.PLAYWRIGHT_BROWSERS_PATH || '(unset — Playwright default cache)';
    console.log('FAIL  the playwright module loaded, but the Chromium build it wants is not in the cache.');
    console.log('');
    console.log('      This is the BROWSER, not the module. The import succeeded.');
    console.log('');
    console.log('      The likely cause is a VERSION MISMATCH, not a missing download. Each');
    console.log('      Playwright release pins one browser revision, and a cache populated by');
    console.log('      a different release satisfies none of it — the cache can be full of');
    console.log('      Chromium and still be the wrong Chromium. Check these two agree first:');
    console.log('');
    console.log('          the resolved module   expected playwright ' + PLAYWRIGHT_VERSION);
    console.log('          the browser cache     ' + cache);
    console.log('');
    console.log('      If they disagree, fix the disagreement: match the pin in');
    console.log('      web/package.json to the release that filled the cache, or point');
    console.log('      PLAYWRIGHT_BROWSERS_PATH at the cache belonging to the pinned version.');
    console.log('');
    console.log('      Only once they agree, and the revision is genuinely absent, download it:');
    console.log('');
    console.log('          pnpm -C web exec playwright install chromium');
    console.log('');
    console.log('      Running that first does make the error go away, by fetching a SECOND');
    console.log('      browser build alongside the one already there. It also buries the');
    console.log('      version skew that caused this, so the next upgrade fails the same way.');
    console.log('');
    console.log('      Playwright said:');
    for (const line of msg.split('\n').slice(0, 6)) console.log('        ' + line.trim());
    process.exit(1);
  } finally {
    cp.spawn = origSpawn;
  }
}

const SCREENS = ['home', 'services', 'libraries', 'search', 'requests'];
const WIDTHS = [390, 1280, 1440, 1680, 1920];
const DENSITIES = ['compact', 'standard', 'relaxed'];

/* The mockups draw two installs and every rendered sweep below covers both.
 *
 * This is not "run it twice for luck". The v0.1 install is where four of the
 * six media types have no catalogue source, so it is the only place several
 * layouts exist at all — a Home Block A row carrying a state and an action
 * instead of a count, a two-library scope chip, a services table with the
 * media servers cut out of it — and a sweep that only ever saw the full stack
 * was blind to every one of them. `full` is first because it is the default
 * the page loads in.
 *
 * Cost: the screen×state corpus roughly doubles, and every floor below is
 * restated against the doubled figure rather than left at a number the new
 * reality satisfies without trying. A floor that cannot fail is the silent
 * pass these floors exist to convert into a failure. */
const INSTALLS = ['full', 'v01'];
const setInstall = async (page, install) => {
  await page.selectOption('#install', install);
  await page.waitForTimeout(40);
};

/* ---------------------------------------------------------------------------
 * PANEL TRAVERSAL, and the hole it closes.
 *
 * Every rendered sweep below used to walk install × screen × state and stop
 * there. Three screens also carry a PANEL SWITCHER — a `[data-group]
 * [data-panel]` control that toggles sibling `[data-panel-for]` blocks — and
 * nothing in this file had ever clicked one. Whatever loads hidden stayed
 * hidden for the whole run, so five blocks of markup were outside every sweep
 * for the entire life of this file:
 *
 *   home     homeview = table | POSTERS
 *   search   viewmode = table | POSTERS
 *   services settings = services | GENERAL | TAGS | UI
 *
 * The two poster grids are the ones that matter, and they are the ones this
 * file most needed to be looking at: they are the only card surface in the
 * product, the only place text was ever set over a computed `dominant_color`
 * fill, and Search's are `data-inst`-scoped on top of being panel-scoped.
 *
 * THE EVIDENCE THAT THE GAP WAS REAL, rather than a theory about the code.
 * Commit a5c9399 added 84 `title=` attributes to poster cards. The §13 copy
 * corpus counts `title` strings, and it counted 74 on the tree before that
 * commit and 74 on the tree after it. A corpus that cannot see 84 new
 * instances of exactly the thing it counts is not measuring the page, and
 * every floor derived from it was a floor over a page with two panels missing.
 * That is also why b417a53's poster-title migration could claim completeness
 * it did not have and pass this gate: Search's ten cards still nested the
 * title inside `.card__art`, and no sweep here had ever rendered them.
 *
 * HOW IT TRAVERSES. `panelsOf` enumerates the switcher controls that are
 * actually reachable in the current install/screen/state — a switcher inside a
 * `[data-when]` block does not exist in every state — and returns the full
 * cross product of the groups it found, so a screen with no switcher costs
 * exactly one pass and nothing changes for it. Every pass names every group
 * explicitly rather than relying on what the previous pass left behind.
 *
 * `resetPanels` puts a screen back to its default before the sweep moves on,
 * because panel selection is DOM state that outlives a state change and a hash
 * route. The default is the FIRST control in DOM order, and that is asserted
 * rather than assumed — see check 8c, which loads the page fresh and fails if
 * the first control is not the one carrying aria-pressed / aria-current true.
 *
 * Trap 3 from the header applies to every click here: a panel control can be
 * an `<a href="#settings">`, following a fragment scrolls the page, and every
 * geometry read in this file is viewport-relative. Both helpers normalise
 * scroll before they return.
 * ------------------------------------------------------------------------- */
const PANEL_CTL = '[data-group][data-panel]';
const panelLabel = (pass) => (pass.length ? ' ' + pass.map((p) => p.group + '=' + p.panel).join(',') : '');

async function panelsOf(page, screen) {
  return page.evaluate(({ s, sel }) => {
    const root = document.querySelector('#pg-' + s);
    if (!root) return [[]];
    const groups = [];
    root.querySelectorAll(sel).forEach((c) => {
      /* Reachable means reachable: a switcher the current state hides is not
         a control the user has, and clicking it would measure a combination
         the product cannot reach. */
      if (c.closest('[hidden]') || !c.offsetParent) return;
      const g = c.getAttribute('data-group');
      let e = groups.find((x) => x.group === g);
      if (!e) groups.push((e = { group: g, panels: [] }));
      const v = c.getAttribute('data-panel');
      if (!e.panels.includes(v)) e.panels.push(v);
    });
    let passes = [[]];
    for (const e of groups) {
      const next = [];
      for (const p of passes) for (const v of e.panels) next.push(p.concat([{ group: e.group, panel: v }]));
      passes = next;
    }
    return passes;
  }, { s: screen, sel: PANEL_CTL });
}

async function setPanels(page, screen, pass) {
  await page.evaluate(({ s, pass }) => {
    for (const p of pass) {
      const el = document.querySelector(
        '#pg-' + s + ' [data-group="' + p.group + '"][data-panel="' + p.panel + '"]');
      if (el) el.click();
    }
    window.scrollTo(0, 0);
  }, { s: screen, pass });
  if (pass.length) await page.waitForTimeout(30);
}

async function resetPanels(page, screen) {
  await page.evaluate(({ s, sel }) => {
    const root = document.querySelector('#pg-' + s);
    if (!root) return;
    const seen = new Set();
    root.querySelectorAll(sel).forEach((c) => {
      const g = c.getAttribute('data-group');
      if (seen.has(g)) return;
      seen.add(g);
      c.click();
    });
    window.scrollTo(0, 0);
  }, { s: screen, sel: PANEL_CTL });
}

let failures = 0;
const fail = (msg) => { failures++; console.log('FAIL  ' + msg); };
const ok = (msg) => console.log('ok    ' + msg);
const note = (msg) => console.log('      ' + msg);
const head = (msg) => console.log('\n== ' + msg + ' ' + '='.repeat(Math.max(0, 68 - msg.length)));

/* ---------------------------------------------------------------------------
 * Source inventory. The five screen files are the source of truth; prototype
 * .html is generated from them by build_prototype.py, so scanning it would
 * double-count every finding and report the generated file's line numbers.
 * ------------------------------------------------------------------------- */
/* The mockup-directory half of the list comes from provenance.mjs rather than
   being spelled out again here. That check asks "is every source this gate
   scans named by a stamp entry?", and a second derivation of "source this gate
   scans" would let the two answers disagree without either of them being
   wrong. */
const SOURCES = [
  join(DESIGN, 'tokens.css'),
  ...mockupSources(MOCKUPS),
];
const rel = (p) => relative(ROOT, p);

/* Strip comments, so a rule never fires on the prose that documents it. Also
 * strips <script type="application/json"> style blocks? No — nothing here uses
 * them. Line count is preserved so reported line numbers stay real. */
function strip(text, kind) {
  const blank = (m) => m.replace(/[^\n]/g, ' ');
  let out = text;
  if (kind === 'html') out = out.replace(/<!--[\s\S]*?-->/g, blank);
  // /* … */ in CSS and JS alike, then // … in JS only (a URL's // must survive).
  out = out.replace(/\/\*[\s\S]*?\*\//g, blank);
  if (kind === 'js') out = out.replace(/(^|[^:'"\\])\/\/[^\n]*/g, (m, p) => p + blank(m.slice(p.length)));
  return out;
}
function kindOf(p) { return p.endsWith('.html') ? 'html' : p.endsWith('.js') ? 'js' : 'css'; }

const FILES = SOURCES.map((p) => {
  const raw = readFileSync(p, 'utf8');
  return { path: p, raw, kind: kindOf(p), src: strip(raw, kindOf(p)) };
});

function lineOf(text, index) { return text.slice(0, index).split('\n').length; }

/* Scan every stripped source for a regex. Returns
 * [{file, line, text, groups}].
 *
 * `text` is the readable snippet the failure list prints — truncated, because a
 * 400-character match in an error message helps nobody. `groups` is the match's
 * NAMED CAPTURE GROUPS, and it is what an exemption is allowed to look at.
 * Nothing else is exposed on purpose; see rule(). */
function scan(re, filter = () => true) {
  const hits = [];
  for (const f of FILES) {
    if (!filter(f)) continue;
    const r = new RegExp(re.source, re.flags.includes('g') ? re.flags : re.flags + 'g');
    let m;
    while ((m = r.exec(f.src)) !== null) {
      hits.push({
        file: rel(f.path),
        line: lineOf(f.src, m.index),
        text: m[0].trim().slice(0, 90),
        groups: m.groups || {},
      });
      if (m.index === r.lastIndex) r.lastIndex++;
    }
  }
  return hits;
}

/* =============================================================================
 * EXEMPTIONS ARE DATA, AND A CHECK THAT MATCHES NOTHING IS A FAILURE
 *
 * Two mechanisms live here, and both exist because of the same defect class:
 * a check that silently does nothing reads exactly like a check that passed.
 *
 * 1. EXEMPTIONS ARE DATA MATCHED AGAINST A NAMED CAPTURE GROUP.
 *
 *    `rule()` used to take a free-form `allow(hit)` predicate. The
 *    text-align:center rule used it to exempt dialogs — and it tested
 *    `hit.text`, which held the matched DECLARATION (`"text-align: center"`)
 *    and never the selector. The exemption could not fire under any input. It
 *    was latent only because nothing centred existed to exempt; the first
 *    legitimate centred dialog would have failed the rule, and the likeliest
 *    repair is deleting the rule rather than fixing the harness.
 *
 *    So the shape changed rather than the one call site. An exemption is now
 *    `{ group, match, why }`: the regex MUST declare a named capture group of
 *    that name, and the exemption tests that group's text. If the group is not
 *    in the pattern, `rule()` throws at startup — a predicate that cannot see
 *    what it claims to test is no longer expressible, so the audit becomes a
 *    property of the design instead of a sweep somebody has to remember to
 *    repeat. Every rule also reports how many hits its exemption absorbed, so
 *    an exemption that stops firing is visible rather than silent.
 *
 * 2. EVERY STATIC CHECK DECLARES A FLOOR AND FAILS BELOW IT.
 *
 *    A regex that matches nothing because a file moved, a glob went stale or a
 *    selector was renamed prints the same `ok` as a genuine pass — so the
 *    check's most reassuring output was its least trustworthy. Each static
 *    check now states roughly how many things it expects to be LOOKING AT, and
 *    finding far fewer is a failure. The count is printed against the floor so
 *    the number is visible, not just the verdict.
 *
 *    The floor is on the corpus, never on the violations: a ban rule wants
 *    zero hits and some minimum amount of material scanned.
 * ========================================================================== */

/* The whole stripped corpus is ~700k characters across 9 files, and it grew by
 * roughly a third when the mockups gained their second install: the five screen
 * files each carry the v0.1 variant of everything that differs. The floor moves
 * with it. 200k was slack against the old corpus and is trivial against this
 * one, and a floor that cannot fail is the silent pass these floors exist to
 * convert into a failure. 480k is still far below today's figure, so ordinary
 * editing never trips it, and far above what losing a whole screen file would
 * leave. A rule that narrows the corpus with `filter` states its own floor. */
const CORPUS_FLOOR = 480000;

/* Assert that a static check actually looked at something. `n` is what it
 * examined, `floor` the least that is credible. */
function floorOk(label, n, floor, unit) {
  if (n < floor) {
    fail(`${label} — scanned only ${n} ${unit}, below the floor of ${floor}. ` +
      `A check that matches nothing is not a check that passed: something moved, ` +
      `renamed or stopped being parsed.`);
    return false;
  }
  return true;
}

/* A named static rule: fails with every hit listed, passes with a count.
 *
 * opts:
 *   filter  (file) => boolean         which sources to scan
 *   exempt  {group, match, why}       data, not a predicate; see above
 *   floor   number                    least credible corpus size, in chars
 *                                     scanned; defaults to the whole corpus
 *                                     being non-trivial */
function rule(name, re, opts = {}) {
  const { filter = () => true, exempt = null, floor = CORPUS_FLOOR } = opts;

  /* The corpus this rule actually looked at. A rule whose filter excludes
   * everything is the silent-no-op case, and it is caught here. */
  const scanned = FILES.filter(filter);
  const chars = scanned.reduce((n, f) => n + f.src.length, 0);
  if (!floorOk(name, chars, Math.max(floor, 1), 'characters of source')) return 1;

  if (exempt) {
    /* A named group that does not exist in the pattern would make the
     * exemption unreachable, which is the bug this mechanism exists to
     * prevent. Fail at startup, loudly, rather than at the first real hit. */
    if (!new RegExp(re.source).source.includes(`(?<${exempt.group}>`)) {
      throw new Error(
        `check.mjs is mis-wired: rule "${name}" declares an exemption on the ` +
        `named group "${exempt.group}", but its pattern has no (?<${exempt.group}>…) ` +
        `group. The exemption could never fire.`);
    }
  }

  const all = scan(re, filter);
  const hits = [];
  let exempted = 0;
  for (const h of all) {
    if (exempt && exempt.match.test(h.groups[exempt.group] ?? '')) { exempted++; continue; }
    hits.push(h);
  }

  const tail = exempt
    ? `  (${exempted} exempt: ${exempt.why})`
    : '';
  if (hits.length) {
    fail(`${name} — ${hits.length} hit(s)${tail}`);
    for (const h of hits.slice(0, 8)) note(`${h.file}:${h.line}  ${h.text}`);
    if (hits.length > 8) note(`… and ${hits.length - 8} more`);
  } else {
    ok(`${name}  [${scanned.length} file(s), ${chars} chars scanned]${tail}`);
  }
  return hits.length;
}

/* =============================================================================
 * 1. §13 ban list
 * ========================================================================== */
head('1. DESIGN-DIRECTION §13 ban list (static, comments stripped)');
note(`${FILES.length} source files: ` + FILES.map((f) => rel(f.path).replace('docs/design/', '')).join(' '));

/* --- colour ------------------------------------------------------------- */
/* The four families §13 names, plus the five CSS keywords that ARE those
 * families under another name: `orchid` (#da70d6), `plum` (#dda0dd) and
 * `magenta` (#f0f, the `fuchsia` synonym), then the family's two pale tints,
 * `thistle` (#d8bfd8) and `lavender` (#e6e6fa). The tints are the reason this
 * list cannot be replaced by a chroma floor: at C 0.0439 and C 0.0269 they sit
 * below any floor that does not also ban neutrals, so the word rule is the
 * only thing that reaches them. Word-banning all five costs nothing — `\b`
 * already declines `plumbing`, `plummet`, `magentaBright`, `lavenderblush`,
 * `thistledown`, `whistle` and `bristle`, and no non-colour use of any of the
 * five exists anywhere in docs/. */
rule('§13 colour: no indigo/violet/purple/fuchsia/orchid/plum/magenta/lavender/thistle',
  /\b(indigo|violet|purple|fuchsia|orchid|plum|magenta|lavender|thistle)\b/i);
rule('§13 colour: no gradients or bg-clip-text', /\b(linear-gradient|radial-gradient|conic-gradient|bg-gradient|bg-clip-text)\b/i);
rule('§13 colour: no pure black or white literals',
  /(#fff\b|#ffffff\b|#000\b|#000000\b|\bcolor:\s*(white|black)\b|\bbackground(-color)?:\s*(white|black)\b|\brgba?\(\s*0\s*,\s*0\s*,\s*0\b|\brgba?\(\s*255\s*,\s*255\s*,\s*255\b)/i);
rule('§13 colour: no text-shadow, no hued drop-shadow', /\b(text-shadow|drop-shadow\(\s*[^)]*(rgb|hsl|#[0-9a-f]{3}))/i);

/* --- typography --------------------------------------------------------- */
/* Structural exclusion #2: a font ban is a font-stack ban. It is evaluated
 * over the family list of a font-family / --font-* declaration, matched as a
 * whole family name. "Internally" and "The Zone of Interest" are prose and are
 * not reachable from here. */
function banFonts() {
  const BANNED = ['inter', 'geist', 'space grotesk', 'instrument serif', 'poppins'];
  const decl = /(?:font-family|--font-[a-z-]+|--fs-family)\s*:\s*([^;{}]+)/gi;
  let stacks = 0, bad = [];
  for (const f of FILES) {
    let m;
    while ((m = decl.exec(f.src)) !== null) {
      stacks++;
      for (const fam of m[1].split(',')) {
        const name = fam.trim().replace(/^["']|["']$/g, '').toLowerCase();
        if (BANNED.includes(name)) bad.push(`${rel(f.path)}:${lineOf(f.src, m.index)}  ${name}`);
      }
    }
  }
  /* A parser that stops matching declarations reports "no banned family" —
     which is the vacuous pass this floor exists to convert into a failure.
     Two families x (tokens.css + usarr.css) plus the component stacks is
     comfortably above 6. */
  if (!floorOk('§13 type: font stacks', stacks, 6, 'font-family declaration(s)')) return;
  if (bad.length) { fail('§13 type: banned family in a font stack'); bad.forEach((b) => note(b)); }
  else ok(`§13 type: no banned family in any font stack (${stacks} font-family declarations parsed, floor 6, families matched whole)`);
}
banFonts();

rule('§13 type: no Google Fonts reference', /fonts\.(googleapis|gstatic)\.com/i);
rule('§13 type: no --text-empty / --fs-empty token', /--(text|fs)-empty\b/);
rule('§13 type: no font-size token above 20px', /--(?:text|fs)-[a-z0-9]+\s*:\s*(2[1-9]|[3-9]\d|\d{3,})px/);
rule('§13 type: no uppercase transform on a label, no italic heading',
  /text-transform\s*:\s*uppercase|h[1-6][^{}]*\{[^}]*font-style\s*:\s*italic/i);
/* §9.6: centred text is legal in a dialog and nowhere else.
 *
 * The `where` group is the mechanism, not decoration: the exemption is about
 * WHERE the declaration sits, so the pattern has to capture the selector (CSS)
 * or the element and its attributes (an inline style) and hand THAT to the
 * exemption. The previous version matched the bare declaration and exempted on
 * it, which could never fire — see the note above rule(). Two arms, because
 * the two positions look nothing alike:
 *   · `…selector… { …decls… text-align: center`  — a CSS rule
 *   · `<div class="dialog__foot" style="…text-align:center`  — inline */
const CENTER = /(?<where>[^{}]{0,200}\{[^{}]{0,400}?|<[a-z][^<>]{0,300}?)text-align\s*:\s*center/i;
/* ⚠️ NARROWED 2026-08-17, from /dialog|modal|toast/ to /dialog|modal/. §13
 * names ONE component — "no text-align: center outside dialog components" —
 * and `modal` is that component under another word, so it stays. `toast` is a
 * DIFFERENT component: it arrived by regex convenience, nobody has argued for
 * centring one, and §9.4 specifies toasts without asking for it. The exemption
 * is inert today, which is precisely the reason to narrow it now rather than
 * later — an unused exemption grants everything on the day someone builds the
 * component, and it grants it silently, because the rule it disables never
 * fired while the component did not exist. An exemption is a claim about a
 * component that has been argued; this one had not been. */
rule('§13 type: no text-align:center outside dialog', CENTER, {
  exempt: { group: 'where', match: /dialog|modal/i, why: '§9.6 allows centring inside a dialog' },
});
rule('§13 type: no bordered / filled empty state',
  /\.empty[a-z-]*[^{}]*\{[^}]*(border\s*:|border-style\s*:\s*dashed|background\s*:|box-shadow\s*:)/i);

/* --- layout ------------------------------------------------------------- */
rule('§13 layout: no backdrop-filter', /backdrop-(filter|blur)/i);
rule('§13 layout: no arbitrary-value class syntax', /class="[^"]*\[[^\]]*(px|rem|#[0-9a-f]{3})[^\]]*\]/i);
/* This check used to scan for `border-radius: Npx` ONLY, and the mockups use
 * `border-radius: var(--radius-0)` everywhere — so it matched ZERO declarations
 * and printed a pass on every run. The ceiling it claims to enforce lives in
 * the --radius-* token definitions, which its pattern never looked at. Same
 * defect family as the content-visibility guard and the dead dialog exemption:
 * assert the effect, not the declaration. The pattern now covers both the
 * literal property and the token definitions that feed it, and the floor makes
 * a return to zero a failure rather than a pass. */
(function radius() {
  const hits = scan(/(?:border-radius|--radius[a-z0-9-]*)\s*:\s*([0-9.]+)px/gi);
  const over = hits.filter((h) => parseFloat(h.text.match(/([0-9.]+)px/)[1]) > 6);
  if (!floorOk('§13 layout: border-radius ceiling', hits.length, 3, 'radius value(s)')) return;
  if (over.length) { fail('§13 layout: border-radius above the 6px ceiling'); over.forEach((h) => note(`${h.file}:${h.line}  ${h.text}`)); }
  else ok(`§13 layout: every radius ≤ 6px (${hits.length} value(s) across literal border-radius and --radius-* token definitions, floor 3)`);
})();

/* --- iconography -------------------------------------------------------- */
/* Structural exclusion #1: §13 bans these as ICON IMPORTS. Two positions are
 * icon positions in this codebase — an ES import specifier, and the mockup's
 * own vocabulary of <symbol id="i-…">. A string compared against
 * KeyboardEvent.key is neither, so it is not excluded, it is out of scope. */
function banIcons() {
  const BANNED = ['Sparkles', 'Zap', 'Shield', 'BarChart3', 'ArrowRight'];
  const ALLOWED_IDS = ['menu', 'search', 'sun', 'moon', 'check', 'alert', 'x-circle',
    'dash-circle', 'chevron-down', 'chevron-right', 'download', 'external', 'refresh'];
  let imports = 0, ids = new Set(), bad = [];
  for (const f of FILES) {
    let m;
    const imp = /import\s+(?:type\s+)?\{([^}]*)\}\s*from|import\s+([A-Za-z_$][\w$]*)\s+from/g;
    while ((m = imp.exec(f.src)) !== null) {
      const names = (m[1] || m[2] || '').split(',').map((s) => s.trim().split(/\s+as\s+/)[0].trim()).filter(Boolean);
      imports += names.length;
      for (const n of names) if (BANNED.includes(n)) bad.push(`${rel(f.path)}:${lineOf(f.src, m.index)}  import ${n}`);
    }
    const sym = /<symbol\s+id="i-([a-z0-9-]+)"/g;
    while ((m = sym.exec(f.src)) !== null) ids.add(m[1]);
  }
  for (const id of ids) {
    if (!ALLOWED_IDS.includes(id)) bad.push(`icon id "i-${id}" is not on the §13 allowlist — a new icon is a PR discussion`);
    if (BANNED.some((b) => b.toLowerCase() === id.replace(/-/g, ''))) bad.push(`icon id "i-${id}" is a banned icon`);
  }
  /* The mockups have no icon imports at all, so `imports` is legitimately 0 and
     the allowlist over <symbol id> is where this rule's teeth are. The floor is
     therefore on the ids, not on the imports. When this check is later pointed
     at web/src/** the import arm carries the rule instead and the vocabulary
     becomes per-target; the floor moves with it. */
  if (!floorOk('§13 icons: icon vocabulary', ids.size, 8, 'icon id(s)')) return;
  if (bad.length) { fail('§13 icons: banned or unlisted icon'); bad.forEach((b) => note(b)); }
  else ok(`§13 icons: ${ids.size} icon ids all on the allowlist (floor 8), ${imports} import specifier(s) clean ` +
    `(bans evaluated on import specifiers and <symbol id> only, so a KeyboardEvent.key never enters)`);
}
banIcons();

rule('§13 icons: no emoji codepoints in app source', /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{FE0F}]/u, {
  /* the mockup's own ⚠/✓ are SVG symbols; prose files are docs */
  filter: (f) => f.kind !== 'html',
  /* Floor: the css and js sources together. If this ever drops the rule is
     scanning nothing and would pass silently. */
  floor: 20000,
});

/* --- motion ------------------------------------------------------------- */
/* Same defect as radius(), and found the same way: every transition in the
 * mockups is `var(--dur-…)`, so a pattern that only read literal
 * transition/animation declarations matched ZERO and passed vacuously. The
 * --dur-* token definitions are where the 200ms ceiling actually lives. */
(function motion() {
  const durs = scan(/(?:(?:transition|animation)(?:-duration)?|--dur[a-z0-9-]*)\s*:\s*[^;{}]*?([0-9.]+)m?s/gi);
  const over = durs.filter((h) => {
    const m = h.text.match(/([0-9.]+)(ms|s)\b/);
    if (!m) return false;
    return (m[2] === 's' ? parseFloat(m[1]) * 1000 : parseFloat(m[1])) > 200;
  });
  if (!floorOk('§13 motion: duration ceiling', durs.length, 4, 'duration value(s)')) return;
  if (over.length) { fail('§13 motion: transition-duration above 200ms'); over.forEach((h) => note(`${h.file}:${h.line}  ${h.text}`)); }
  else ok(`§13 motion: ${durs.length} duration(s) across transition/animation declarations and --dur-* token definitions, none above 200ms (floor 4)`);

  const bez = scan(/cubic-bezier\(([^)]*)\)/gi);
  const bad = bez.filter((h) => {
    const n = h.text.replace(/[^0-9.,-]/g, '').split(',').map(Number);
    return n.length === 4 && (n[1] < 0 || n[1] > 1 || n[3] < 0 || n[3] > 1);
  });
  if (bad.length) { fail('§13 motion: cubic-bezier overshoots on Y'); bad.forEach((h) => note(`${h.file}:${h.line}  ${h.text}`)); }
  else ok(`§13 motion: ${bez.length} cubic-bezier(s), no control point outside 0..1 on Y`);
})();

rule('§13 motion: no transition on a non-composited property',
  /transition\s*:\s*(?![^;{}]*\b(opacity|transform|color|background|border-color|outline)\b)[^;{}]*\b(height|width|top|left|right|bottom|margin|padding|box-shadow|background-position)\b/i);
rule('§13 motion: no startViewTransition', /startViewTransition/);
rule('§13 motion: no IntersectionObserver reveal', /IntersectionObserver/);
(function reducedMotion() {
  const has = FILES.filter((f) => /prefers-reduced-motion\s*:\s*reduce/.test(f.src));
  if (!has.length) fail('§13 motion: no prefers-reduced-motion: reduce block anywhere');
  else ok(`§13 motion: prefers-reduced-motion block present in ${has.map((f) => rel(f.path)).join(', ')}`);
})();

/* --- controls ----------------------------------------------------------- */
(function outlineNone() {
  /* §13 states this one precisely: `outline: none` is legal only when the very
   * next rule targets the SAME selector with :focus-visible and restores ≥2px. */
  let total = 0, bad = [];
  for (const f of FILES) {
    const re = /([^{}]+):focus\s*\{[^}]*outline:\s*(?:none|0)[^}]*\}([\s\S]{0,400})/g;
    let m;
    while ((m = re.exec(f.src)) !== null) {
      total++;
      const sel = m[1].trim().replace(/[\s\S]*[};]/, '').trim();
      const next = m[2].replace(/^\s*/, '');
      const okNext = new RegExp('^' + sel.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') +
        ':focus-visible\\s*\\{[^}]*outline:\\s*(?!none|0)\\d*\\.?\\d+(px|rem|em)').test(next);
      if (!okNext) bad.push(`${rel(f.path)}:${lineOf(f.src, m.index)}  ${sel}:focus { outline: none } with no immediately-following :focus-visible restore`);
    }
    /* the unconditional form, anywhere else */
    const loose = /outline\s*:\s*(none|0)\s*[;}]/g;
    while ((m = loose.exec(f.src)) !== null) {
      const before = f.src.slice(Math.max(0, m.index - 300), m.index);
      if (!/:focus\s*\{[^}]*$/.test(before)) {
        bad.push(`${rel(f.path)}:${lineOf(f.src, m.index)}  outline removed outside the :focus/:focus-visible pair`);
      }
    }
  }
  if (bad.length) { fail('§13 controls: outline removed without a compliant restore'); bad.forEach((b) => note(b)); }
  else ok(`§13 controls: ${total} :focus{outline:none} declaration(s), every one immediately restored on :focus-visible`);
})();

rule('§13 controls: everything that navigates is an <a href>', /<div[^>]*\sonclick=/i);
/* The sharpest vacuous-pass shape in the file: "every page has a skip link" is
 * trivially true of zero pages, so if the html glob ever stops matching this
 * prints a pass while checking nothing. There are five screen files and the
 * floor says so. */
(function skipLink() {
  const pages = FILES.filter((f) => f.kind === 'html');
  if (!floorOk('§13 controls: skip link', pages.length, 5, 'screen file(s)')) return;
  const missing = pages.filter((f) => !/skip[^<]{0,20}to[^<]{0,20}content/i.test(f.raw));
  if (missing.length) { fail('§13 controls: no "Skip to content" link'); missing.forEach((f) => note(rel(f.path))); }
  else ok(`§13 controls: every one of the ${pages.length} screen files carries a Skip to content link (floor 5)`);
})();

/* =============================================================================
 * 1c. The two-install invariant
 *
 * Two attributes own the `hidden` flag on these screens and they must never
 * both own it on the same element: `applyState` writes `hidden` on every
 * `[data-when]` in the current screen, and `paintInstall` writes it on every
 * `[data-inst]` in the document. An element carrying both has two writers and
 * one attribute, and the symptom is not a crash — it is a block that quietly
 * reappears in a state it does not belong to, which is exactly the class of
 * defect a reader of a mockup cannot distinguish from a design decision.
 *
 * They compose by NESTING instead, which costs one wrapper and is checkable
 * here in six lines. The floor is on the number of `data-inst` elements found:
 * if the mechanism is ever renamed, this rule finds nothing and would pass in
 * silence.
 * ========================================================================== */
head('1c. data-when and data-inst never write hidden on the same element');
(function installInvariant() {
  const html = FILES.filter((f) => f.kind === 'html');
  const tags = [];
  for (const f of html) {
    for (const m of f.src.matchAll(/<[a-zA-Z][^>]*>/g)) {
      if (/\bdata-inst=/.test(m[0])) tags.push({ file: rel(f.path), tag: m[0] });
    }
  }
  if (!floorOk('install invariant', tags.length, 60, 'data-inst element(s)')) return;
  const both = tags.filter((t) => /\bdata-when=/.test(t.tag));
  if (both.length) {
    fail(`install invariant: ${both.length} element(s) carry both data-when and data-inst`);
    both.slice(0, 6).forEach((t) => note(`${t.file}  ${t.tag.slice(0, 110)}`));
  } else {
    ok(`install invariant: ${tags.length} data-inst elements (floor 60), none of them also carrying data-when`);
  }
  /* Every install-scoped element must name an install the switcher offers, or
   * it renders in neither and is dead markup nobody will ever see fail. */
  const VALID = new Set(['full', 'v01']);
  const wrong = tags.filter((t) => {
    const v = /\bdata-inst="([^"]*)"/.exec(t.tag);
    return !v || !v[1].split(/\s+/).filter(Boolean).length ||
      v[1].split(/\s+/).filter(Boolean).some((x) => !VALID.has(x));
  });
  if (wrong.length) {
    fail(`install invariant: ${wrong.length} data-inst value(s) name no install the switcher offers`);
    wrong.slice(0, 6).forEach((t) => note(`${t.file}  ${t.tag.slice(0, 110)}`));
  } else {
    ok(`install invariant: every data-inst value is one of ${[...VALID].join(', ')}`);
  }
})();


/* =============================================================================
 * 1d. CSP: the mockups never carry an inline style attribute
 *
 * §14's production Content-Security-Policy drops inline styles. A mockup that
 * demonstrates layout through a style attribute is therefore demonstrating
 * something that CANNOT SHIP: it renders correctly here and would render
 * differently in the real app. That is worse than rendering wrong, because
 * nobody re-checks a screen that already looks right — which is why this is a
 * check and not a review note. 119 of them were removed in one pass; the rule
 * exists so the 120th never lands.
 *
 * WHAT REPLACED THEM, so a reader does not resolve a failure by writing the
 * declaration back in a different disguise:
 *   · genuine layout nudges  -> the .u-* classes in usarr.css §2.13
 *   · --cols / --row-lines   -> the per-list .cols-* classes in §2.12. These
 *     are DATA, not styling, and §7.4 requires --row-lines be declared per
 *     list from that list's own rendered rows — so they are one class per
 *     list and are deliberately not collapsed onto shared values.
 *
 * NOT BANNED, and the distinction is the CSP's own: `el.style.setProperty(…)`
 * from usarr.js. CSP governs the style ATTRIBUTE in markup and the <style>
 * element; CSSOM mutation from script is not an inline style and is not
 * blocked. The poster grid's --dc assignment is that, and it stays. (--dc-fg
 * went with §9.2's move of the poster title off the fill; --dc remains,
 * because it is still the image placeholder.)
 * This rule is therefore evaluated over the HTML sources only.
 * ========================================================================== */
head('1d. CSP: no inline style attribute in the five screen files');
(function noInlineStyle() {
  const html = FILES.filter((f) => f.kind === 'html');
  /* Two floors, because the two ways this rule goes quiet are different. A
     ban rule wants zero hits, so its own hit count can never be the floor —
     the floor is on the corpus. FILE floor: SOURCES globs the mockup
     directory, and a screen file that is renamed or moved simply stops being
     scanned. CHARACTER floor: the pass line prints the live total on every
     run, so this comment does not carry a second copy of it -- the last one
     went stale, and a floor's derivation that restates a number the gate
     already reports is a number with no way to stay true.

     WHAT THE DERIVATION NEEDS IS THE SMALLEST FILE, not the largest. The
     regression is losing one whole screen file, and what has to fall under the
     floor is the REMAINDER the other four leave -- so the case that decides
     whether this floor fires is the one leaving the LARGEST remainder, which
     is losing the SMALLEST file. Losing the largest is the case most certain
     to trip, which is the opposite of binding. Put as a rule: the check
     catches the loss of file f exactly when f exceeds the slack, so the
     ceiling on slack is min(f), not max(f).

     ARITHMETIC, measured 2026-08-22, and not a fired drill. The five stripped:
     requests 126,603 · libraries 123,778 · search 111,733 · index 107,348 ·
     services 99,507, totalling 568,969. The ceiling is therefore `services.html`
     at 99,507, and the slack this floor carries is 568,969 - 500,000 = 68,969.
     68,969 is under 99,507 with 30,538 to spare, so losing any ONE of the five
     trips this check -- the cheapest one to lose included, which is the claim
     the old wording never actually computed. */
  if (!floorOk('§14 CSP: inline style', html.length, 5, 'screen file(s)')) return;
  rule('§14 CSP: no style= attribute in a screen file', /\s(?<attr>style)\s*=\s*["']/, {
    filter: (f) => f.kind === 'html',
    floor: 500000,
  });
  /* prototype.html is generated from the five and is the file that actually
     gets opened and published, so it is asserted too — separately, because it
     is not in SOURCES and a finding here means build_prototype.py went stale
     rather than that somebody edited a screen. */
  const proto = strip(readFileSync(join(MOCKUPS, 'prototype.html'), 'utf8'), 'html');
  if (!floorOk('§14 CSP: prototype.html', proto.length, 400000, 'characters of generated source')) return;
  const protoHits = [...proto.matchAll(/\s(?:style)\s*=\s*["']/g)];
  if (protoHits.length) {
    fail(`§14 CSP: prototype.html carries ${protoHits.length} inline style attribute(s) — ` +
      `regenerate it with build_prototype.py, and if it is already current then ` +
      `the builder is injecting them`);
  } else {
    ok(`§14 CSP: prototype.html clean too [${proto.length} chars scanned, floor 400000]`);
  }
})();

/* =============================================================================
 * 1e. Provenance: prototype.html was generated from these exact source bytes
 *
 * The check, the reasoning behind it and — most importantly — the three things
 * it does NOT prove all live in docs/design/provenance.mjs, which is also its
 * own entry point. It was moved out of this file so it could run WITHOUT a
 * browser: `await loadChromium()` above is top-level, so nothing in here is
 * reachable without one, and this check needs no page, no network and no
 * Chromium. It is now the first step of `make check-offline` as well as running
 * here, where it belongs because every rendered check below opens
 * prototype.html and a stale one makes all of them measure the wrong file.
 *
 * Read the header of that file before changing anything about the stamp. In
 * particular: build_prototype.py writes the format and provenance.mjs reads it,
 * and a third implementation of it is not allowed.
 * ========================================================================== */
head('1e. Provenance: prototype.html is in step with its sources');
checkProvenance({ mockups: MOCKUPS, root: ROOT, fail, ok, note });

/* =============================================================================
 * 2. Token drift — tokens.css is canonical, the mockup carries a copy
 * ========================================================================== */
head('2. Token drift: docs/design/tokens.css vs the mockup copy');

function cssVars(text, blockRe) {
  const m = text.match(blockRe);
  if (!m) return {};
  const out = {};
  for (const d of m[1].matchAll(/(--[a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{3,8})\s*;/g)) out[d[1]] = d[2].toLowerCase();
  return out;
}
const tokensRaw = strip(readFileSync(join(DESIGN, 'tokens.css'), 'utf8'), 'css');
const mockRaw = strip(readFileSync(join(MOCKUPS, 'usarr.css'), 'utf8'), 'css');
const tokLight = cssVars(tokensRaw, /:root\s*\{([\s\S]*?)\n\}/);
const tokDark = cssVars(tokensRaw, /:root\[data-theme="dark"\]\s*\{([\s\S]*?)\n\}/);
const mockLight = cssVars(mockRaw, /:root\s*\{([\s\S]*?)\n\}/);
const mockDark = cssVars(mockRaw, /:root\[data-theme="dark"\]\s*\{([\s\S]*?)\n\}/);

/* usarr.css names differ from tokens.css names by design (the mockup is one
 * flat layer, tokens.css is a ramp plus semantic aliases). The mapping is the
 * contract between the two files, so it lives here rather than in either. */
const MAP = {
  '--n-0': '--bg', '--n-1': '--surface', '--n-2': '--selected', '--n-3': '--border',
  '--n-4': '--border-hi', '--n-5': '--fg-faint', '--n-6': '--fg-muted', '--n-8': '--fg',
  '--status-ok': '--st-ok', '--status-warn': '--st-warn',
  '--status-error': '--st-err', '--status-unset': '--st-none',
};
for (const [theme, a, b] of [['light', tokLight, mockLight], ['dark', tokDark, mockDark]]) {
  const bad = [];
  let n = 0;
  for (const [t, u] of Object.entries(MAP)) {
    if (!(t in a) || !(u in b)) { bad.push(`${theme}: ${t} or ${u} missing`); continue; }
    n++;
    if (a[t] !== b[u]) bad.push(`${theme}: tokens.css ${t} ${a[t]} ≠ usarr.css ${u} ${b[u]}`);
  }
  if (bad.length) { fail(`token drift (${theme})`); bad.forEach((x) => note(x)); }
  else ok(`token drift ${theme}: all ${n} shared values identical in both files`);
}

/* =============================================================================
 * 3. Contrast — worst of all five grounds, both themes
 * ========================================================================== */
head('3. Contrast: worst of all five grounds, both themes');

const lin = (c) => { c /= 255; return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; };
function luminance(hex) {
  const h = hex.replace('#', '');
  const v = h.length === 3 ? [...h].map((c) => parseInt(c + c, 16)) : [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16));
  return 0.2126 * lin(v[0]) + 0.7152 * lin(v[1]) + 0.0722 * lin(v[2]);
}
const contrast = (a, b) => {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
};

/* Floors are DESIGN-DIRECTION §3 / tokens.css targets, which sit at or above
 * the WCAG floor. Status is TEXT: 4.5, not 3. */
const FLOORS = {
  '--fg': 12, '--fg-muted': 5.5, '--fg-faint': 4.5, '--border-hi': 3.2, '--focus': 3,
  '--st-ok': 4.5, '--st-warn': 4.5, '--st-err': 4.5, '--st-none': 4.5,
};
const GROUNDS = ['--bg', '--surface', '--hover', '--selected', '--inset'];

for (const [theme, vars] of [['light', mockLight], ['dark', mockDark]]) {
  const grounds = Object.fromEntries(GROUNDS.map((g) => [g, vars[g]]));
  const missing = GROUNDS.filter((g) => !grounds[g]);
  if (missing.length) { fail(`contrast ${theme}: ground token(s) missing: ${missing.join(' ')}`); continue; }
  let bad = 0;
  for (const [fg, floor] of Object.entries(FLOORS)) {
    if (!vars[fg]) { fail(`contrast ${theme}: ${fg} not defined`); bad++; continue; }
    const ratios = GROUNDS.map((g) => [g, contrast(vars[fg], grounds[g])]);
    const worst = Math.min(...ratios.map((r) => r[1]));
    const line = `${theme} ${fg.padEnd(12)} ${vars[fg]}  ` +
      ratios.map(([g, r]) => `${g.slice(2)} ${r.toFixed(2)}`).join(' · ') +
      `  worst ${worst.toFixed(2)} (floor ${floor})`;
    if (worst < floor) { fail(line); bad++; } else note(line);
  }
  if (!bad) ok(`contrast ${theme}: ${Object.keys(FLOORS).length} foregrounds × ${GROUNDS.length} grounds all clear their floor`);
}

/* tokens.css is audited on its own three grounds — it does not define --inset
 * or a hover fill distinct from --n-1, and that gap is a recorded open
 * question in the file itself, not a defect this check can invent a value for. */
{
  const g = ['--n-0', '--n-1', '--n-2'];
  const F = { '--n-4': 3.2, '--n-5': 4.5, '--n-6': 5.5, '--n-7': 9, '--n-8': 12,
    '--status-ok': 4.5, '--status-warn': 4.5, '--status-error': 4.5, '--status-unset': 4.5 };
  for (const [theme, vars] of [['light', tokLight], ['dark', tokDark]]) {
    let bad = 0;
    for (const [fg, floor] of Object.entries(F)) {
      const worst = Math.min(...g.map((x) => contrast(vars[fg], vars[x])));
      if (worst < floor) { fail(`tokens.css ${theme} ${fg} ${vars[fg]} worst ${worst.toFixed(2)} < ${floor}`); bad++; }
    }
    if (!bad) ok(`tokens.css ${theme}: ${Object.keys(F).length} foregrounds × 3 declared grounds all clear their floor`);
  }
}

/* =============================================================================
 * The rendered checks. One browser, reused.
 * ========================================================================== */
const { browser, spawned: launchedBinaries } = await launchChromium();

/* --- 0. the build that actually launched --------------------------------- */
head('0. chromium.launch() resolves the headless shell, not the full browser');
/* `chromium.executablePath()` reports the FULL build no matter what, so it
 * cannot answer this — it reports chromium-<rev>/chrome-linux/chrome while the
 * process that starts is chromium_headless_shell-<rev>/chrome-linux/
 * headless_shell. The distinction is not cosmetic: the shell is the build with
 * no window, no compositor surface and no GPU path, and it is the one every
 * measurement in this file assumes. A run that silently fell back to the full
 * browser would measure a different renderer from every other machine this has
 * ever been run on, and nothing else here would notice. */
{
  const chrome = launchedBinaries.filter((p) => /chrom|headless_shell/i.test(p));
  if (!chrome.length) {
    fail(`headless shell: no chromium-looking binary was spawned at all (saw ${launchedBinaries.length} spawn(s))`);
  } else if (chrome.every((p) => /chromium_headless_shell/.test(p))) {
    ok(`headless shell: launch() spawned ${chrome[0]}`);
  } else {
    fail('headless shell: launch() spawned the full browser build, not chromium_headless_shell\n' +
      chrome.map((p) => '        ' + p).join('\n'));
  }
}

async function statesOf(page, screen) {
  return page.$$eval('#pg-' + screen + ' [data-act="state"] option', (os) => os.map((o) => o.value));
}

/* --- 4. overflow --------------------------------------------------------- */
head('4. Overflow: nothing past the viewport, nothing past its own .tablewrap');
/* 5 screens × 37 states × 2 installs, each state expanded over the panel
 * switchers it actually offers = 110 combinations per width. The floor is 104:
 * below that a state option has gone missing, an install stopped switching, a
 * panel switcher stopped being enumerated, or the sweep silently ran over one
 * install twice.
 *
 * It was 70 against 74 combinations while no panel was ever opened. The 36 new
 * combinations are the poster panels on Home and Search and the three extra
 * Settings panes on Services, in both installs — which is 36 combinations of
 * this product that this check had never rendered. */
const COMBO_FLOOR = 104;
for (const width of WIDTHS) {
  const ctx = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await ctx.newPage();
  await page.goto(URL);
  let worstOver = 0, worstWhere = '', combos = 0;
  for (const install of INSTALLS) {
  await setInstall(page, install);
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(60);
    for (const state of await statesOf(page, screen)) {
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(40);
      for (const pass of await panelsOf(page, screen)) {
      combos++;
      await setPanels(page, screen, pass);
      const where = `${width}px ${install}/${screen}/${state}${panelLabel(pass)}`;
      const r = await page.evaluate((s) => {
        const main = document.querySelector('#pg-' + s);
        let over = 0, worst = 0; const ex = [], clip = [];
        main.querySelectorAll('*').forEach((el) => {
          const b = el.getBoundingClientRect();
          if (!b.width || b.right <= window.innerWidth + 0.5) return;
          over++;
          if (b.right > worst) worst = b.right;
          if (ex.length < 4) ex.push((el.className || el.tagName) + ' r=' + Math.round(b.right) +
            ' "' + (el.textContent || '').trim().slice(0, 30) + '"');
        });
        main.querySelectorAll('.tablewrap').forEach((w) => {
          if (w.closest('[hidden]') || !w.offsetParent) return;
          const edge = w.getBoundingClientRect().right;
          w.querySelectorAll('*').forEach((el) => {
            if (el.closest('[hidden]')) return;
            const b = el.getBoundingClientRect();
            if (!b.width || b.right <= edge + 0.5) return;
            if (clip.length < 4) clip.push((el.className || el.tagName) + ' +' +
              Math.round(b.right - edge) + 'px past the wrapper "' + (el.textContent || '').trim().slice(0, 30) + '"');
          });
        });
        return { over, worst: Math.round(worst), ex, clip, doc: document.documentElement.scrollWidth };
      }, screen);
      if (r.clip.length) fail(`${where}: sheared by overflow-x:clip on .tablewrap\n        ` + r.clip.join('\n        '));
      if (r.over) {
        worstOver = Math.max(worstOver, r.worst - width);
        worstWhere = `${where}: ${r.over} over, worst right=${r.worst}\n        ${r.ex.join('\n        ')}`;
      }
      if (r.doc > width) fail(`${where}: document scrolls sideways (${r.doc})`);
      }
    }
    await resetPanels(page, screen);
  }
  }
  if (!floorOk(`overflow at ${width}px`, combos, COMBO_FLOOR, 'screen×state×install×panel combination(s)')) {
    /* already failed */
  } else if (worstOver > 0) fail('overflow ' + worstWhere);
  else ok(`overflow at ${width}px: ${combos} screen×state×install×panel combinations (floor ${COMBO_FLOOR}), nothing past the viewport or its wrapper`);
  await ctx.close();
}

/* --- the rest, at the reference desktop width ---------------------------- */
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
await page.goto(URL);
await page.waitForTimeout(200);

/* --- 5. row heights against the density bands ---------------------------- */
head('5. Row heights against the three density bands');
/* Services and Libraries carry the tight ceiling: they are the two screens the
 * row-height finding was filed against, and a cell there holding prose is the
 * defect. Home, Search and Requests are held looser — their tallest rows are a
 * track listing and a release row carrying a two-line confirmation, which is
 * content doing work rather than prose that escaped into a cell.
 * The ceiling is stated at COMPACT and shifts with the density's own --row-h,
 * so the band moves with the setting rather than being three hand-written
 * tables that can disagree. */
const CEILING_COMPACT = { services: 49, libraries: 49, home: 60, search: 80, requests: 80 };

/* The ceilings above are stated AT COMPACT, so the shift for the other two
 * densities is measured against compact's own --row-h. That baseline used to be
 * the literal 28, which silently hardcoded a token: if --row-h at compact ever
 * moved, every band would still be computed from 28 and would print numbers
 * that look like measurements and are not. It is read from the token now, and
 * asserted, because a wrong baseline is worse than an error. */
const COMPACT_ROW_H_EXPECTED = 28;
await page.evaluate(() => { document.documentElement.setAttribute('data-density', 'compact'); });
const COMPACT_ROW_H = await page.evaluate(() =>
  parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--row-h')));
if (!Number.isFinite(COMPACT_ROW_H) || COMPACT_ROW_H <= 0) {
  fail(`rows: --row-h did not resolve at compact (got ${COMPACT_ROW_H}), so every density band ` +
    `below would be computed from nothing. The token or the density attribute has moved.`);
} else if (COMPACT_ROW_H !== COMPACT_ROW_H_EXPECTED) {
  fail(`rows: --row-h at compact is ${COMPACT_ROW_H}px, but CEILING_COMPACT is written against ` +
    `${COMPACT_ROW_H_EXPECTED}px. The ceilings are stated at compact, so they must be restated ` +
    `for the new value rather than shifted from a stale baseline.`);
} else {
  ok(`rows: --row-h at compact reads ${COMPACT_ROW_H}px from the token, matching the baseline ` +
    `CEILING_COMPACT is written against`);
}

/* Rows measured per screen, over both installs, every state and every panel.
 * The floor is per screen because the screens differ by an order of magnitude —
 * Home renders hundreds of rows across its states, Libraries tens — and one
 * global floor would be satisfied by Home alone while Libraries silently
 * measured nothing.
 *
 * n counts ROW RENDERINGS, not distinct rows, and panel traversal raised it on
 * the two screens that have a switcher: Home 205 -> 274 and Services 65 -> 221,
 * with Libraries, Search and Requests unmoved because they have no panel. Most
 * of that rise is the same table measured once per panel pass rather than new
 * markup — the poster panels contain no <tr> at all. Re-measuring an unchanged
 * table costs nothing and misreports nothing: this check asserts a min, a
 * median and a max against a band, none of which a duplicate moves. The floors
 * are restated against the new figures anyway, because a floor left at 180 over
 * a population of 274 is a floor that cannot fail.
 *
 * SERVICES FELL 221 -> 170 on 18 Aug 2026, and the floor is restated rather
 * than the drawing padded. ADR-0041 made Kavita v0.1's one catalogue source, so
 * the v0.1 health table went from five *Arr rows to two -- Kavita and Prowlarr,
 * which is every service that milestone connects. Three fewer rows, each with
 * its expander, measured across every state, install and panel pass, is the
 * whole of the 51. This is a smaller drawing, not a check that stopped
 * matching, and the floor is set at the same ~85% of the live population every
 * other screen's is. If it falls again, ask what left the table before lowering
 * it further. */
const ROW_FLOOR = { home: 240, services: 145, libraries: 24, search: 60, requests: 145 };

for (const density of DENSITIES) {
  await page.evaluate((d) => { document.documentElement.setAttribute('data-density', d); }, density);
  const rowH = await page.evaluate(() => parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--row-h')));
  const shift = rowH - COMPACT_ROW_H;
  for (const screen of SCREENS) {
    const hs = [];
    for (const install of INSTALLS) {
      await setInstall(page, install);
      await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
      await page.waitForTimeout(50);
      const states = await statesOf(page, screen);
      for (const state of states) {
        if (state === 'annex') continue; /* labelled as documentation for services neither install has */
        await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
        await page.waitForTimeout(30);
        for (const pass of await panelsOf(page, screen)) {
          await setPanels(page, screen, pass);
          hs.push(...await page.evaluate((s) => [...document.querySelectorAll('#pg-' + s + ' tbody tr')]
            .filter((r) => !r.hidden && !r.closest('[hidden]') && r.offsetParent)
            .map((r) => Math.round(r.getBoundingClientRect().height)).filter((h) => h > 0), screen));
        }
      }
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', states[0]);
      await resetPanels(page, screen);
    }
    hs.sort((a, b) => a - b);
    const ceiling = CEILING_COMPACT[screen] + shift;
    const line = `rows ${density.padEnd(9)} ${screen.padEnd(10)} n=${String(hs.length).padStart(3)}  ` +
      `min=${hs[0]}  median=${hs[Math.floor(hs.length / 2)]}  max=${hs[hs.length - 1]}  ` +
      `band ${rowH}–${ceiling}px  floor n≥${ROW_FLOOR[screen]}`;
    if (!floorOk(`rows ${density} ${screen}`, hs.length, ROW_FLOOR[screen], 'rendered row(s)')) continue;
    if (hs[hs.length - 1] > ceiling) fail(line + `   <-- above the ${ceiling}px ceiling`);
    else if (hs[0] < rowH - 0.5) fail(line + `   <-- below the ${rowH}px --row-h floor`);
    else ok(line);
  }
}
await setInstall(page, 'full');
await page.evaluate(() => { document.documentElement.setAttribute('data-density', 'compact'); });

/* --- 6. availability accessible names ------------------------------------ */
head('6. Availability glyphs carry a word');
{
  let bad = 0, total = 0;
  /* THE PANEL TRAVERSAL IS WHY THIS CHECK CAN SEE THE POSTER CARDS AT ALL,
     and that is the history worth keeping here: before any panel was opened
     this swept 165 of 195, because the poster card carries an .avail of its
     own and 42 of them across the two grids had never once been looked at.

     THE FLOOR IS DERIVED, NOT ROUNDED, and the live population is left to the
     pass line rather than copied here -- the figure that used to sit in this
     comment was true when written and had drifted by 15 before anyone
     re-read it. THE DRIFT WAS AUTHORED, NOT A REGRESSION, and it is recorded
     so the next reader does not chase it again: `c666382`, the ADR-0041
     re-draw of the mockups' v0.1 install to Kavita and Prowlarr, moved all 15
     -- Home's v01 grid lost a row of eight, and Search's v01 media types
     became ones this design gives no .avail (Ebooks and Audiobooks carry
     none; Comics, Movies and TV do). That commit moved ROW_FLOOR.services and
     wrote a rider for it while moving this population silently, which is the
     failure mode a floor that restates a count invites: 320 still passed, so
     nothing turned red.

     THE REGRESSION THIS FLOOR IS SIZED AGAINST. A screen leaving SCREENS, or
     panel traversal going away, shows up in the combination count COMBO_FLOOR
     watches -- 104 against a live 110 -- so this floor is not sized against
     either. What it is sized against is the .avail population shrinking while
     the traversal still runs in full, which the combination count cannot see
     because the count does not change. That much WAS fired: the drill below
     left all 110 combinations intact and COMBO_FLOOR green.

     THE UNIT OF THAT LOSS IS A CELL, NOT A SCREEN, and getting that wrong is
     what left the previous derivation short. Measured 2026-08-22 by
     attributing every reading to its install AND its screen: full/home 248,
     full/search 27, v01/home 72, v01/search 13, the other six cells none --
     Services, Libraries and Requests carry no .avail at all. A screen does not
     draw the same population in both installs, so "the whole of Search's
     contribution", 40, is two losses added together rather than the cheapest
     one. The cheapest is the smallest nonzero install-and-screen cell,
     v01/search at 13. That is not a hypothetical class: `c666382`, the commit
     this comment already documents, moved 15 readings by changing what the
     v01 install draws, which is exactly this shape of loss.

     So the ceiling on slack is 13, and slack must be STRICTLY below it. THE
     RULE FOR SETTING A FLOOR IS THE SMALLEST VALUE THAT SATISFIES ITS
     CEILING, never the largest: a higher floor catches nothing extra and
     spends authoring headroom that ordinary editing needs. 348 leaves 12, one
     under the ceiling, and is the smallest value that does. (The rule is for
     DERIVING a floor, not a licence to re-lower one that already satisfies.)

     FIRED BOTH WAYS, 2026-08-22, by hiding every .avail in v01/search and
     leaving the sweep otherwise whole: the population read 347, exactly 13
     down, with all 110 combinations still swept and COMBO_FLOOR green. At the
     old 330 that ran `ok` and the whole gate exited 0; at 348 it fails. The
     320 before it left exactly 40 against a ceiling it had mis-derived as 40.

     WHAT THIS FLOOR STILL DOES NOT SEE, said rather than implied. The sweep's
     unit is finer than a cell -- it is install x screen x state x panel -- and
     the smallest nonzero one of those, measured the same day, is 1:
     full/search/typed and v01/search/typed contribute a single reading each.
     A regression costing one state's .avail on one screen in one install is
     under this floor, and no floor on a population of 360 reaches it. This
     value bounds how cheap a caught loss can be; it is not a claim that
     nothing cheaper exists. */
  const AVAIL_FLOOR = 348;
  for (const install of INSTALLS) {
    await setInstall(page, install);
    for (const screen of SCREENS) {
      await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
      await page.waitForTimeout(50);
      for (const state of await statesOf(page, screen)) {
        await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
        await page.waitForTimeout(30);
        for (const pass of await panelsOf(page, screen)) {
          await setPanels(page, screen, pass);
          const r = await page.evaluate((s) => {
            let bad = 0, total = 0;
            document.querySelectorAll('#pg-' + s + ' .avail').forEach((el) => {
              if (el.closest('[hidden]')) return;
              total++;
              if (!el.textContent.trim()) bad++;
            });
            return { bad, total };
          }, screen);
          bad += r.bad; total += r.total;
        }
      }
      await resetPanels(page, screen);
    }
  }
  await setInstall(page, 'full');
  if (bad) fail(`${bad} of ${total} .avail elements have an empty accessible name`);
  else if (floorOk('availability', total, AVAIL_FLOOR, 'rendered .avail element(s)')) {
    ok(`availability: all ${total} rendered .avail elements carry a word (floor ${AVAIL_FLOOR}, both installs)`);
  }
}

/* --- 7. one tab stop per list -------------------------------------------- */
head('7. One tab stop per list (roving tabindex, or a declared opt-out)');
{
  let checked = 0, bad = 0, nonTable = 0;
  /* 78 while the corpus was tables-only and no panel was ever opened. Both
     floors below are restated against what the traversal actually finds, and
     the non-table floor is separate on the same argument the copy corpus makes
     per source: one combined floor would be satisfied by the tables alone
     while the card grids silently contributed nothing, which is exactly the
     condition this check was in for its whole life. */
  const LIST_FLOOR = 120;
  const NON_TABLE_FLOOR = 10;
  for (const install of INSTALLS) {
  await setInstall(page, install);
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    const states = await statesOf(page, screen);
    for (const state of states) {
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(30);
      for (const pass of await panelsOf(page, screen)) {
      await setPanels(page, screen, pass);
      const where = `${install}/${screen}/${state}${panelLabel(pass)}`;
      const r = await page.evaluate((s) => {
        const out = [];
        /* THE CORPUS IS EVERY ROVING LIST, NOT EVERY TABLE. It used to be
           `table[role="table"]` alone, which meant the four poster grids —
           `.grid[data-roving=".card"]`, two on Home and two on Search — were
           outside the check that exists to guarantee a list is one tab stop.
           They were doubly outside it: not a table, and inside a panel nothing
           ever opened. The roving model in usarr.js is declared by attribute
           and says nothing about tables, so the corpus is declared the same
           way. `[data-roving-optout]` is collected too, so an opt-out is a
           thing this check has SEEN and dismissed rather than a thing it never
           found. */
        const root = document.querySelector('#pg-' + s);
        const seen = new Set([
          ...root.querySelectorAll('table[role="table"]'),
          ...root.querySelectorAll('[data-roving]'),
          ...root.querySelectorAll('[data-roving-optout]'),
        ]);
        for (const t of seen) {
          if (t.closest('[hidden]') || t.hidden) continue;
          const isTable = t.tagName === 'TABLE';
          /* The declared selector is the list's own definition of an item; the
             `tbody tr` fallback is for a table that declares nothing, which is
             the defect the `inner` branch below hunts. */
          const sel = t.getAttribute('data-roving') || 'tbody tr';
          const rows = [...t.querySelectorAll(sel)].filter((x) => !x.hidden && x.offsetParent);
          if (!rows.length) continue;
          out.push({
            label: t.getAttribute('aria-label'),
            kind: isTable ? 'table' : t.tagName.toLowerCase() + (t.className ? '.' + t.className.split(/\s+/)[0] : ''),
            items: rows.length,
            optout: t.hasAttribute('data-roving-optout'),
            roving: t.hasAttribute('data-roving'),
            /* Focusables the list would hand the user as extra tab stops. For
               a table that is what the cells hold; for a card grid the item
               IS the link, so its own descendants are what count and there
               are none. */
            inner: isTable
              ? t.querySelectorAll('tbody a[href], tbody button, tbody input, tbody select').length
              : rows.reduce((n, r) => n + r.querySelectorAll('a[href], button, input, select').length, 0),
            zero: rows.filter((x) => x.tabIndex === 0).length,
          });
        }
        return out;
      }, screen);
      for (const l of r) {
        checked++;
        if (l.kind !== 'table') nonTable++;
        if (l.optout) continue;
        if (l.inner > 0 && !l.roving) { bad++; fail(`roving: ${where} "${l.label}" (${l.kind}) has ${l.inner} focusable descendants, no data-roving and no declared opt-out`); }
        else if (l.roving && l.zero !== 1) { bad++; fail(`roving: ${where} "${l.label}" (${l.kind}, ${l.items} items) has ${l.zero} items at tabindex 0`); }
      }
      }
    }
    await page.selectOption('#pg-' + screen + ' [data-act="state"]', states[0]);
    await resetPanels(page, screen);
  }
  }
  await setInstall(page, 'full');
  if (!bad && floorOk('roving', checked, LIST_FLOOR, 'list rendering(s)') &&
      floorOk('roving non-table', nonTable, NON_TABLE_FLOOR, 'non-table list rendering(s)')) {
    ok(`roving: every rendered list is one tab stop or declares why not (${checked} list renderings ` +
      `checked, floor ${LIST_FLOOR}; ${nonTable} of them card grids rather than tables, floor ` +
      `${NON_TABLE_FLOOR}; both installs, every panel)`);
  }
}

/* --- 8. containment is live on the row primitive ------------------------- */
head('8. content-visibility actually does something on a row');
/* THE CHECK THAT WOULD HAVE CAUGHT THE <tr> MISTAKE IMMEDIATELY.
 *
 * CSS Containment Level 2 excludes internal table boxes from size, layout and
 * paint containment, and `content-visibility: auto` is defined entirely in
 * terms of those three. On a `display: table-row` element the declaration
 * parses, reads back as `auto`, and does nothing at all — which is invisible in
 * the source and invisible in a screenshot. The only way to see it is to
 * measure: a container whose off-screen rows are skipped reports a DIFFERENT
 * scrollHeight from the same container with the rows fully laid out.
 *
 * The list is cloned up to 400 rows first, because with eight rows on screen
 * nothing is off-screen and the two measurements agree for a legitimate reason.
 * That would make this assertion pass on sample-data size rather than on the
 * mechanism, which is precisely the failure mode it exists to avoid.
 *
 * The clone is given a DELIBERATELY WRONG contain-intrinsic-size (200px against
 * a ~28px row) so the signal is unambiguous. Measuring the shipped estimate
 * instead would compare the placeholder against the real height and pass or
 * fail on how good the estimate is, which is a different question — §7.4's
 * scroll-drift budget owns that one, and it needs `make bench`. This check owns
 * only "does the declaration do anything at all", and a placeholder 7x the real
 * height makes that a several-thousand-pixel difference or exactly zero.
 *
 * The control run is the point. The same manipulation is repeated with the rows
 * forced back to `display: table-row`, and it MUST produce no difference. If it
 * ever does, this assertion has stopped testing containment and is measuring
 * something else. */
{
  await page.evaluate((s) => { window.location.hash = '#' + s; }, 'search');
  await page.waitForTimeout(80);
  const r = await page.evaluate(() => {
    const src = document.querySelector('#pg-search table.tbl');
    if (!src) return { err: 'no .tbl on Search' };

    function measure(forceDisplay) {
      const host = document.createElement('div');
      host.style.cssText = 'position:absolute;left:-99999px;top:0;width:1200px;height:600px;overflow:auto';
      const t = src.cloneNode(true);
      const tbody = t.querySelector('tbody');
      const proto = [...tbody.querySelectorAll('tr')].filter((x) => !x.hidden)[0];
      if (!proto) return null;
      tbody.textContent = '';
      for (let i = 0; i < 400; i++) tbody.appendChild(proto.cloneNode(true));
      host.appendChild(t);
      document.body.appendChild(host);
      const rows = [...tbody.children];
      const shippedCV = getComputedStyle(rows[0]).contentVisibility;
      const shippedDisplay = getComputedStyle(rows[0]).display;
      rows.forEach((x) => {
        if (forceDisplay) x.style.display = forceDisplay;
        x.style.contentVisibility = 'auto';
        x.style.containIntrinsicSize = 'auto 200px';
      });
      host.scrollTop = 0; void host.offsetHeight;
      const contained = host.scrollHeight;
      rows.forEach((x) => { x.style.contentVisibility = 'visible'; });
      void host.offsetHeight;
      const uncontained = host.scrollHeight;
      const display = getComputedStyle(rows[0]).display;
      host.remove();
      return { display, shippedCV, shippedDisplay, contained, uncontained, rows: rows.length };
    }
    return { live: measure(null), control: measure('table-row') };
  });

  if (r.err) fail('containment: ' + r.err);
  else {
    const { live, control } = r;
    if (live.shippedCV !== 'auto') {
      fail(`containment: the shipped row primitive does not declare content-visibility:auto (computed "${live.shippedCV}")`);
    } else if (live.contained === live.uncontained) {
      fail(`containment: content-visibility is INERT on the row primitive — display:${live.display}, ${live.rows} rows, ` +
        `scrollHeight ${live.contained}px contained and ${live.uncontained}px uncontained, identical against a 200px placeholder. ` +
        `An internal table box takes no size, layout or paint containment (css-contain-2 §2), so the declaration parses, ` +
        `reads back as "auto", and does nothing. This is the <tr> mistake.`);
    } else {
      ok(`containment: live on the row primitive — shipped display:${live.shippedDisplay}, content-visibility:${live.shippedCV}, ` +
        `${live.rows} rows, scrollHeight ${live.contained}px contained vs ${live.uncontained}px uncontained (Δ ${Math.abs(live.contained - live.uncontained)}px)`);
    }
    if (control.contained !== control.uncontained) {
      fail(`containment control: forcing display:table-row still changed scrollHeight ` +
        `(${control.contained} vs ${control.uncontained}) — this assertion is no longer measuring containment`);
    } else {
      ok(`containment control: the same 400 rows forced to display:table-row give ${control.contained}px either way ` +
        `(Δ 0px), so the assertion above has teeth`);
    }
  }
}

/* --- 8b. the install switcher actually switches --------------------------- */
head('8b. The install switcher: two installs, and every screen answers to it');
/* Without this, every sweep above could run twice over an identical page and
 * report double the coverage for none. It asserts three separate things:
 *
 *   · the switcher is in the SHARED CHROME, so it survives a hash route and
 *     exists on all five screens rather than on whichever one drew it;
 *   · `full` is the state the page LOADS in, because the default view has to
 *     be the six-populated-type stack the design is judged on;
 *   · switching changes the rendered text of every one of the five screens.
 *     A screen that does not move is a screen the second sweep did not test.
 */
{
  const sel = await page.$('#install');
  if (!sel) fail('install: no #install switcher in the shared chrome');
  else {
    const opts = await page.$$eval('#install option', (os) => os.map((o) => ({ v: o.value, t: o.textContent.trim() })));
    const values = opts.map((o) => o.v);
    if (values.join(',') !== INSTALLS.join(',')) {
      fail(`install: the switcher offers [${values.join(', ')}], the sweeps run [${INSTALLS.join(', ')}]`);
    } else {
      ok(`install: the switcher offers exactly the two installs the sweeps run — ` +
        opts.map((o) => `${o.v} "${o.t}"`).join(', '));
    }
    /* The switcher is not a product control and there is no setting it
     * corresponds to, so it has to sit INSIDE the mockup notice. Drawn in the
     * top bar proper it would fabricate a product affordance, which is the one
     * thing the notice exists to stop the rest of the prototype doing. */
    const inNotice = await page.$eval('#install', (s) => !!s.closest('.mocknote'));
    if (!inNotice) fail('install: the switcher sits outside the mockup notice, so it reads as a product control');
    else ok('install: the switcher sits inside the mockup notice, not in the product chrome');

    /* The milestone has to be in the LABEL, not only in the notice: the label
     * is what a reader sees while looking at the screen it describes. */
    const unlabelled = opts.filter((o) => !/v0\.1|milestone/i.test(o.t));
    if (unlabelled.length) {
      fail(`install: ${unlabelled.length} option label(s) name a stack without naming a milestone: ` +
        unlabelled.map((o) => `"${o.t}"`).join(', '));
    } else {
      ok('install: both option labels place their stack against a milestone rather than naming it alone');
    }

    await page.reload();
    await page.waitForTimeout(200);
    const loaded = await page.$eval('#install', (s) => s.value);
    if (loaded !== 'full') fail(`install: the page loads in "${loaded}", but the default view must be the full stack`);
    else ok('install: the page loads in the full stack, which is the default the design is judged on');

    let moved = 0;
    for (const screen of SCREENS) {
      /* Over EVERY state, not only the one the screen loads in. Requests
       * differs between the two installs inside `nosink` and nowhere else, so
       * a probe that read the default state alone declared a screen carrying
       * six install-scoped strings "identical" — which is the false reassurance
       * this whole check exists to remove. */
      const read = async (install) => {
        await setInstall(page, install);
        await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
        await page.waitForTimeout(60);
        const states = await statesOf(page, screen);
        let all = '';
        for (const state of states) {
          await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
          await page.waitForTimeout(25);
          all += state + '\u0000' + await page.$eval('#pg-' + screen, (m) => m.innerText.replace(/\s+/g, ' ')) + '\u0000';
        }
        return all;
      };
      const a = await read('full');
      const b = await read('v01');
      if (a === b) fail(`install: ${screen} renders identically in both installs across all its states, so the second sweep tested nothing new`);
      else moved++;
    }
    await setInstall(page, 'full');
    if (moved === SCREENS.length) ok(`install: all ${moved} screens render differently in the two installs`);

    /* The mockup notice is the labelled-mockup exception DESIGN-DIRECTION rule
     * 13 grants, so it has to stay true of whichever install is selected.
     *
     * Everything below asserts a PROPERTY of the notice, never its wording,
     * and that is the finding rather than the code. The notice used to read
     * "…UsArr, which is pre-alpha software: none of these screens is
     * implemented and every value on them is invented", and half of it went
     * false while nobody noticed — the screens shipped. Any guard that had
     * pinned that sentence VERBATIM would have been holding the false half in
     * place: it would have gone green for as long as the claim stayed wrong
     * and failed the first person to correct it. A guard asserts a property,
     * not a fact; pinning a fact makes the guard an obstacle to fixing it.
     * Three properties, each of which survives any honest rewording:
     *
     *   · the notice describes the install that is selected;
     *   · the notice names its data as invented — the true half, and the whole
     *     reason rule 13 grants the labelled-mockup exception;
     *   · the notice claims nothing about implementation status. That fact
     *     belongs to ARCHITECTURE §16 and to the tree; a mockup that restates
     *     it owns a copy nothing keeps in step, which is REVIEW-LOG SD-01 and
     *     is exactly how the sentence above went stale.
     */
    for (const install of INSTALLS) {
      await setInstall(page, install);
      /* The long form of the notice, not the whole .mocknote — the switcher
       * itself lives inside the notice, so reading the container would compare
       * the option list rather than the sentence. */
      const notice = await page.$eval('.mocknote__long:not([hidden])', (n) => n.innerText.replace(/\s+/g, ' '));
      const claimsFull = /every catalogue source/.test(notice);
      if (install === 'full' ? !claimsFull : claimsFull) {
        fail(`install: the permanent mockup notice does not describe the ${install} install — "${notice.slice(0, 120)}"`);
      } else {
        ok(`install: the mockup notice describes the ${install} install — "${notice.slice(0, 96)}…"`);
      }

      if (!/\b(invent(ed|s)?|fabricated|made up)\b/i.test(notice)) {
        fail(`install: the ${install} mockup notice does not name its data as invented, which is the ` +
          `one thing rule 13's exception is granted for — "${notice.slice(0, 120)}"`);
      } else {
        ok(`install: the ${install} mockup notice names its data as invented`);
      }

      const status = notice.match(/\b(pre-alpha|unimplemented|implemented|shipped|ships)\b/i);
      if (status) {
        fail(`install: the ${install} mockup notice makes an implementation-status claim ("${status[0]}") — ` +
          `§16 and the tree own that fact, a mockup restating it owns a copy that goes stale (SD-01)`);
      } else {
        ok(`install: the ${install} mockup notice makes no implementation-status claim`);
      }
    }

    /* The same property over document.title, and it is here rather than in the
     * §13 corpus below because that corpus COUNTS strings and does not read
     * them. The title carried the identical claim — "UsArr screen mockups:
     * static, invented data, nothing implemented" — for the same reason and
     * for the same length of time, and it is the one user-visible string a
     * rendered DOM walk cannot see, so it is the one most likely to be missed
     * again. One document, one assertion. */
    const docTitle = await page.title();
    const titleStatus = docTitle.match(/\b(pre-alpha|unimplemented|implemented|shipped|ships)\b/i);
    if (titleStatus) {
      fail(`document.title makes an implementation-status claim ("${titleStatus[0]}") — "${docTitle}". ` +
        `§16 and the tree own that fact (SD-01); the tab is not the place to restate it`);
    } else {
      ok(`document.title makes no implementation-status claim — "${docTitle}"`);
    }

    await setInstall(page, 'full');
  }
}

/* --- 8c. the panel switchers, and the traversal's one assumption ---------- */
head('8c. The panel switchers: every panel is reachable, and the default is the first');
/* The traversal above resets a screen by clicking the FIRST `[data-group]
 * [data-panel]` control of each group, which is only the default because the
 * markup happens to put it first. That is an assumption, and an assumption a
 * sweep depends on is a thing to assert, not a thing to comment. This runs on
 * a FRESH page — nothing here has clicked anything — so "at load" means at
 * load.
 *
 * It also prints the inventory, which is the point of check design rule 1: a
 * traversal that silently stopped finding switchers would otherwise look
 * exactly like a traversal that found them all and had nothing to say. */
{
  const pctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const ppage = await pctx.newPage();
  await ppage.goto(URL);
  await ppage.waitForTimeout(150);
  /* These three are set AT today's figures, not below them, and that is the
     one place in this file where that is right. They are an inventory, not a
     population: every sweep above is scoped by what this inventory finds, so a
     switcher that stops being found silently shrinks every corpus below it —
     which is the exact failure being fixed here. Losing a panel should cost
     whoever removes it one line in this file. */
  const GROUP_FLOOR = 3, CTL_FLOOR = 8, PANEL_FLOOR = 8;
  const inv = await ppage.evaluate((sel) => {
    const groups = {};
    document.querySelectorAll(sel).forEach((c) => {
      const g = c.getAttribute('data-group');
      (groups[g] ||= { panels: [], first: null, marked: [] });
      groups[g].panels.push(c.getAttribute('data-panel'));
      if (groups[g].first === null) groups[g].first = c.getAttribute('data-panel');
      if (c.getAttribute('aria-pressed') === 'true' || c.getAttribute('aria-current') === 'true') {
        groups[g].marked.push(c.getAttribute('data-panel'));
      }
    });
    const blocks = {};
    document.querySelectorAll('[data-panel-for]').forEach((p) => {
      const g = p.getAttribute('data-panel-for');
      (blocks[g] ||= []).push({ name: p.getAttribute('data-panel-name'), hidden: p.hidden });
    });
    return { groups, blocks };
  }, PANEL_CTL);

  const names = Object.keys(inv.groups);
  const ctls = names.reduce((n, g) => n + inv.groups[g].panels.length, 0);
  const blocks = Object.values(inv.blocks).reduce((n, b) => n + b.length, 0);
  if (floorOk('panels: groups', names.length, GROUP_FLOOR, 'switcher group(s)') &&
      floorOk('panels: controls', ctls, CTL_FLOOR, 'switcher control(s)') &&
      floorOk('panels: blocks', blocks, PANEL_FLOOR, 'panel block(s)')) {
    ok(`panels: ${names.length} switcher groups, ${ctls} controls, ${blocks} panel blocks — ` +
      names.map((g) => `${g}[${inv.groups[g].panels.join('|')}]`).join(' '));
  }
  const before = failures;
  for (const g of names) {
    const e = inv.groups[g];
    if (e.marked.length !== 1) {
      fail(`panels: group "${g}" has ${e.marked.length} controls marked current at load, so it has no unambiguous default`);
    } else if (e.marked[0] !== e.first) {
      fail(`panels: group "${g}" loads with "${e.marked[0]}" selected but "${e.first}" is first in DOM order — ` +
        `the traversal resets to the first control, so those two must agree`);
    }
    const have = (inv.blocks[g] || []).map((b) => b.name);
    const missing = e.panels.filter((p) => !have.includes(p));
    if (missing.length) fail(`panels: group "${g}" offers [${missing.join(', ')}] with no [data-panel-for="${g}"] block behind it`);
    const shown = (inv.blocks[g] || []).filter((b) => !b.hidden).map((b) => b.name);
    if (shown.length !== 1) fail(`panels: group "${g}" shows ${shown.length} panels at load, not 1`);
  }
  if (failures === before) {
    ok('panels: every group has one default, one visible block at load, and a block behind every control');
  }
  /* The hidden blocks are the whole reason this section exists: a panel that
     loads hidden is invisible to any sweep that does not click for it, and
     these are the ones that were. */
  const dark = Object.entries(inv.blocks).flatMap(([g, b]) => b.filter((x) => x.hidden).map((x) => g + '=' + x.name));
  note(`panels: ${dark.length} block(s) load hidden and are reached only by the traversal — ${dark.join(', ')}`);
  await pctx.close();
}

/* --- 9. webfont ---------------------------------------------------------- */
head('9. The webfont actually resolves');
{
  const face = await page.evaluate(() => {
    const c = document.createElement('canvas').getContext('2d');
    const w = (f) => { c.font = '40px ' + f; return c.measureText('Handgloves 0123456789').width.toFixed(3); };
    return {
      plexSans: w('"IBM Plex Sans"'), plexMono: w('"IBM Plex Mono"'),
      bogus: w('"ZZZNoSuchFaceZZZ"'), uiStack: w(getComputedStyle(document.body).fontFamily),
      loaded: [...document.fonts].map((f) => f.family + ' ' + f.weight + ' ' + f.status).join(' | '),
    };
  });
  if (face.plexSans === face.bogus) fail('webfont: IBM Plex Sans is not resolving (identical width to a bogus family)');
  else ok(`webfont: IBM Plex Sans resolves (${face.plexSans}px vs bogus ${face.bogus}px), body renders at ${face.uiStack}px`);
  if (face.plexMono === face.bogus) fail('webfont: IBM Plex Mono is not resolving');
  else ok(`webfont: IBM Plex Mono resolves (${face.plexMono}px)`);
  note('document.fonts: ' + face.loaded);
}

/* --- 1b. the copy bans, over rendered chrome text ------------------------ */
head('1b. §13 copy bans, over rendered chrome text, cells included');
/* Structural exclusion #3, AND THE ONE THAT USED TO BE A HOLE. The copy rules
 * govern the product's own voice, and some rendered strings are a substituted
 * value rather than something UsArr wrote — "BOOM! Studios" is a publisher.
 *
 * This used to be spelt `<td>`: every string in every cell, text and attribute
 * alike, was outside the corpus. The comment that stood here said the blanket
 * exemption "is only safe because prose is kept out of cells, which is check 5
 * above; the two rules hold each other up." That was false as measured. Check 5
 * is a ROW-HEIGHT ceiling, it is deliberately loose on three of the five screens
 * (CEILING_COMPACT: 60/80/80 on home/search/requests against 49 on services and
 * libraries), and it skips Services' `annex` state outright — none of which is a
 * ban on prose in a cell. Prose was in the cells: dropping the exclusion took the
 * corpus from 6,908 strings to 13,411, and the 6,503 that appeared are what §13
 * had never once been applied to, since 1b was written.
 *
 * IT IS DECLARED PER SITE NOW: `data-copy="data"` on the element holding a
 * substituted value, honoured by both corpora, and COUNTED AND CAPPED — the
 * number is printed beside the corpus total, so an opt-out is a thing this
 * check has SEEN and dismissed rather than a thing it never found, and it is
 * held under a ceiling of its own (OPTOUT_CEILING) so that widening the
 * exclusion fails rather than merely showing up in a figure nobody reads. That is check 7's argument
 * for collecting `[data-roving-optout]`, applied to the same shape of problem.
 * The exemption now rests on the markup saying which value is data, and on
 * nothing else: no other check holds it up, and none is asked to. */
{
  const BANNED_WORDS = ['seamlessly', 'effortlessly', 'powerful', 'simply', 'unlock', 'empower',
    'elevate', 'streamline', 'supercharge', 'robust', 'leverage', 'intuitive', 'blazing',
    'world-class', 'comprehensive', 'ai-powered'];

  /* Structural exclusion #5, and the one that is derived from a document rather
   * than declared here. §13's em-dash rule already carries an exception — "except
   * where ARCHITECTURE.md §17 fixes the wording … §17 wins over this checklist, so
   * the rule carries the exception rather than the banner carrying a rewrite" —
   * and that exception was written as a single hard-coded banner, so the next
   * string §17 fixes would fail the sweep and get rewritten in the mockup instead
   * of being recognised. It is read out of §17 instead. Two consequences worth
   * having: the exemption cannot go stale, and a label that DRIFTS from §17's
   * wording loses its exemption and fails, which turns this into a copy-drift
   * check between the mockups and the section that specifies them. */
  const arch = readFileSync(join(ROOT, 'docs', 'ARCHITECTURE.md'), 'utf8');
  const s17 = arch.slice(arch.indexOf('\n## 17. '));
  const norm = (s) => s.toLowerCase().replace(/[`*_"“”]/g, '').replace(/\s+/g, ' ').trim();
  /* The raw body is named because the §17 sweep below reads it too, and it must
   * be the SAME slice the exemption is granted from — an exemption drawn from
   * one span of the document and a check drawn from another is two rules. */
  const s17Body = s17.slice(0, s17.indexOf('\n## ', 10) === -1 ? undefined : s17.indexOf('\n## ', 10));
  const fixedBy17 = norm(s17Body);
  if (fixedBy17.length < 5000) fail('§13 copy: ARCHITECTURE §17 could not be located, so the fixed-wording exemption is not being applied');
  /* What §17 fixes is the CONSTRUCTION, not the instance. Its banner is quoted
   * as "Kavita is unreachable — showing cached data from 14:02"; the mockup
   * renders the same banner over different sample data, and over a different
   * service — it draws a Radarr. So the exemption is granted on the em dash's
   * own two-words-either-side window, which survives a substituted service name
   * and a substituted timestamp and does not survive a rewritten phrase. ⚠️ The
   * subject moved on 2026-08-17, from `Radarr 4K` to `Kavita`, and the window
   * "is unreachable — showing cached" did not — which is the property this
   * design was chosen for, demonstrated rather than asserted: the mockup's
   * banner kept its exemption across a §17 edit that renamed its subject. */
  const exempt = (t) => {
    const w = norm(t).split(' ');
    for (let i = 0; i < w.length; i++) {
      if (w[i] !== '—') continue;
      const win = w.slice(Math.max(0, i - 2), i + 3).join(' ');
      if (win.split(' ').length >= 3 && fixedBy17.includes(win)) return true;
    }
    return false;
  };

  /* ---------------------------------------------------------------------
   * The corpus, and the hole that used to be in it.
   *
   * This rule read RENDERED TEXT, so it saw only strings that lay out as a
   * block. Every user-visible string that is not laid out at all was outside
   * it, and "outside it" meant §13 had never once been enforced on them:
   *
   *   · document.title — the browser tab, the bookmark, the history entry.
   *     Unreachable from a DOM walk by construction, and it was carrying an
   *     em dash in a nine-word string the whole time.
   *   · aria-label — the accessible NAME. For an icon-only control it is the
   *     entire user-visible string, and the one a screen-reader user hears
   *     instead of the copy this rule was checking.
   *   · title — the tooltip a mouse user gets.
   *   · placeholder — visible until the moment the field is typed into.
   *   · <option> text — an option has no layout box until the menu opens, so
   *     `display` is never blockish and the walk skipped every one of them.
   *     This is how the install switcher's own labels sat outside the sweep.
   *
   * Each source carries its own floor, because one combined floor would be
   * satisfied by aria-label alone while document.title silently contributed
   * nothing — which is precisely the failure that let the title drift.
   *
   * The structural exclusions are the SAME ones the rendered walk uses, and
   * for the same reason: `data-copy="data"` marks a substituted value wherever
   * it is declared, and .statebar is the mockup's own scaffolding. Applying a
   * different exclusion set to attributes would make the two halves of one
   * rule disagree about what counts as the product's voice — which is what the
   * old `<td>` spelling did in reverse, exempting a `title=` on a cell and
   * checking the identical `title=` one element outside it.
   * ------------------------------------------------------------------- */
  const ATTRS = ['aria-label', 'title', 'placeholder', 'alt', 'aria-description',
    'aria-roledescription', 'aria-valuetext', 'aria-placeholder'];
  /* ---------------------------------------------------------------------
   * Per-source floors, EACH DERIVED FROM A NAMED REGRESSION rather than
   * rounded down from whatever the corpus happened to measure that day.
   *
   * WHAT THESE FLOORS ARE NOT THE GUARD FOR. Two things, and they belong to
   * two other guards. A sweep that stops visiting combinations is
   * COMBO_FLOOR's business: that floor is 104 against a live 110 the pass line
   * prints on every run, and it watches the sweep's EXTENT where these watch
   * the collector's SCOPE. Losing a screen, losing panel traversal or losing
   * an install each drops the count under 104 -- by arithmetic over the
   * decomposition measured 2026-08-22, where libraries is the smallest screen
   * at 8 of the 110, panel traversal accounts for 56, and an install is half.
   * THAT ARITHMETIC IS NOT A FIRED DRILL and is not offered as one. And a
   * WIDENED EXCLUSION is OPTOUT_CEILING's business, for the reason set out
   * under "IT IS NOT A CEILING ON THE CHEAPEST LOSS" below: it is
   * element-granular, so it goes under any floor these could carry. This block
   * used to claim that half of the class and could not deliver it.
   *
   * WHICH CLAIMS HERE REST ON A DRILL: the four ceilings below, the two
   * results in the next paragraph, and the exclusion drill under "IT IS NOT A
   * CEILING". Everything else in this block is arithmetic over a measured
   * decomposition, labelled as such where it appears.
   *
   * WHAT THESE ADD, over a corpus floor that counts the very same strings.
   * STRING_FLOOR reads attribute strings too, so it is NOT blind to the scope
   * list below losing one of its three elements (`#pg-<screen>`, `.topbar`,
   * `.sidebar`) -- but it has 211 of slack, and that is what decides which of
   * those losses it can see. Both halves were fired 2026-08-22:
   *
   *   TOO SMALL TO MOVE IT. Dropping .sidebar from the scope list cost 110
   *   strings. STRING_FLOOR stayed GREEN, 110 sitting well inside its 211,
   *   and `title` was the only failure in the run. Without a floor of its own
   *   on that source, the regression ships green.
   *
   *   BIG ENOUGH TO MOVE IT, BUT UNLOCALIZED. Dropping .topbar cost 880.
   *   STRING_FLOOR did fire, 12531 against 13200 -- but all it can report is
   *   that something lost 880 strings. The three per-source lines that failed
   *   beside it named aria-label, placeholder and option, and THAT is what
   *   identifies .topbar as the element that went.
   *
   * So these floors catch the losses too small to move the corpus floor past
   * its own slack, and they localize the ones large enough to move it.
   *
   * THE CEILING, THEREFORE, IS SCOPE-LIST ELEMENT LOSS -- `.topbar` or
   * `.sidebar` leaving the scope list -- and that is the whole of what these
   * four numbers measure. The cost is the source's per-combination
   * contribution from that element multiplied by the 110 combinations swept.
   * Measured 2026-08-22, per combination: `aria-label` 2 from .topbar, `title`
   * 1 from .sidebar, `placeholder` 1 from .topbar, `option` 5 from .topbar --
   * so 220, 110, 110 and 550 over the sweep. Those four numbers are the ceiling
   * on each floor's slack, and all four were fired rather than argued: removing
   * .topbar from the scope list drops aria-label, placeholder and option by
   * exactly 220, 110 and 550, and removing .sidebar drops title by exactly 110.
   *
   * IT IS NOT A CEILING ON THE CHEAPEST LOSS OF THE SOURCE, and this comment
   * used to say it was. The sentence was false, and it was false against the
   * other half of the regression class it claimed to cover: a widened
   * EXCLUSION is element-granular by construction -- `data-copy="data"` goes
   * on one element and only that element -- so its cost is that element's
   * strings times only the combinations that element renders in. Libraries is
   * 8 of the 110, and one state of one screen is fewer still. FIRED
   * 2026-08-22: one `.topbar` button marked `data-copy="data"` took aria-label
   * from 691 to 581 and the whole gate exited 0, against a ceiling this block
   * stated as 220. RAISING THESE FLOORS WOULD NOT HAVE FIXED IT -- a floor
   * satisfying a 110-combination ceiling is still blind at 8, and no floor on
   * a corpus this size reaches 8 at all. The exclusion is gated by
   * OPTOUT_CEILING below instead, on the opt-out COUNT, where the cost does
   * not depend on how many combinations the marked element renders in.
   *
   * SLACK IS DELETION TOLERANCE -- corpus minus floor, the count that can go
   * missing before the floor fires -- and it must be STRICTLY BELOW that
   * ceiling. A floor with more slack than its ceiling sits green through
   * exactly the regression it exists for, which is what 340 was doing against
   * a ceiling of 110.
   *
   * IT IS A MAXIMUM, NOT A TARGET. Two of these already satisfy the criterion
   * with room and are LEFT WHERE THEY ARE ON PURPOSE. Do not tidy them
   * upward: a floor pushed closer to its ceiling catches nothing extra and
   * costs false positives on ordinary editing.
   *
   * AND WHEN AN HONEST AUTHORED CHANGE TURNS ONE RED, the move is NOT to read
   * the new live count off the pass line and round down under it. That is the
   * practice this block exists to end, and it is exactly what the next person
   * to meet a red here will reach for, because it is the fastest thing that
   * makes the line green. Re-derive the ceiling from the regression the floor
   * is named against, set the floor to the SMALLEST value whose slack is
   * strictly under that ceiling, and write the derivation at the site. The two
   * tightest have the least room to absorb a wrong answer: `placeholder`'s 43
   * covers ONE whole-of-Services `placeholder=` deletion at 40 -- Services is
   * 40 of the 110 combinations, and at least four placeholders render in every
   * one of them -- and not a second; `title`'s 93 covers FOUR whole-of-Home
   * deletions at 22 each, Home being 22 of the 110, and not a fifth. Both are
   * arithmetic over the combination decomposition measured 2026-08-22, not
   * fired drills.
   *
   * AND NO FLOOR HERE RESTATES TODAY'S COUNT. The pass line prints the live
   * figure on every run, so a baseline copied into this comment is a second
   * copy with no way to stay true -- which is precisely what happened to the
   * five that used to sit here, four of them stale by the time they were
   * deleted. STRING_FLOOR's comment below carries the long form of that
   * argument; this block follows it.
   *
   * ===================================================================
   * WHAT THIS BLOCK DOES NOT COVER. Written down, because the arrangement
   * above reads like a clean division of labour between COMBO_FLOOR and these
   * floors and it is not one: the two are not a partition of the regression
   * space, and the criterion does not maintain itself. Three known holes, each
   * measured 2026-08-22 on this branch and none of them fixed here.
   *
   *   1. THE CRITERION DECAYS, AND NOTHING WATCHES IT. Slack is `live -
   *   floor` with the floor fixed and the corpus growing, so every floor here
   *   drifts toward its ceiling under ordinary authoring and crosses it in
   *   silence -- no line goes red when a floor stops satisfying the rule it
   *   was set by. This is general to the block; §17 is where it bites first,
   *   because its ceiling is the smallest. Its span population went 71 at
   *   `1bc400a3a` (2026-08-19) to 78 at `70192dcd2` (2026-08-21): +7 in two
   *   days, against a ceiling of 7. Today's slack of 3 therefore lapses in
   *   roughly two days of that rate.
   *
   *   2. THERE IS A GAP BETWEEN THE TWO GUARDS, and a regression sits in it.
   *   COMBO_FLOOR carries 6 of slack. A state disappearing from a screen's
   *   `[data-act="state"]` select costs its panel-pass count once per install,
   *   and the cheapest -- a state running a single panel pass, which every
   *   screen has -- costs 2 combinations. 108 is green at 104. The per-source
   *   losses that ride along are green too: across all 27 single-panel states,
   *   the WORST any of them costs is aria-label 28, option 104, placeholder 12
   *   and title 74, against floors of 191, 218, 43 and 93. Neither guard sees
   *   that regression, at its cheapest (home/empty, 18 strings) or its worst.
   *
   *   3. THE §17 MEDIAN DOES NOT COVER THE SMALL SUBSECTIONS, and the two
   *   regressions it bundles are not distributed alike. Slice truncation is a
   *   SUFFIX loss and 17.8 is last in document order, so its cheapest case is
   *   25 -- far over the floor's slack of 3, and comfortably caught. A
   *   subsection dropping out of the MIDDLE is as little as 1: 17.3.1
   *   contributes one string, 17.3.2 and 17.7 three each, and any of those
   *   three can vanish green. THE FLOOR STAYS AT 75 REGARDLESS, on measured
   *   history rather than on comfort. Enumerating `git log --first-parent
   *   --full-history -- docs/ARCHITECTURE.md` gives 89 commits, of which 18
   *   moved the §17 span population and every one moved it UP; over the full
   *   `--full-history` enumeration, 303 commits, the same count taken against
   *   each commit's OWN parent gives 49 moves, again all upward and not one
   *   decrease anywhere in the history. Buying coverage of a loss that has
   *   never happened, with false positives on a section under active
   *   authorship, is a bad trade -- but that is a bet on the past and not a
   *   property of the floor, which is the point of writing it here.
   * ===================================================================
   * ------------------------------------------------------------------- */
  const CORPUS_FLOORS = {
    /* Nothing to derive: there is exactly one document, so the floor is the
       whole corpus and the slack is zero. It is the smallest floor here and
       the one that matters most, since 0 was the status quo. */
    'document.title': 1,
    /* Regression: .topbar leaves the scope list. Cost 2 x 110 = 220.
       Slack 191, under it. Was 400, whose 291 of slack sat over the ceiling
       and would have held green through the whole loss. */
    'aria-label': 500,
    /* Regression: .sidebar leaves the scope list. Cost 1 x 110 = 110.
       Slack 93, under it.

       WAS 340, WHICH IS THE SLACKEST FLOOR THIS BLOCK HAS EVER CARRIED: 803
       of slack against a ceiling of 110, wide enough to lose the sidebar
       seven times over and still report ok. It was set by rounding down from
       a live count, which is the practice this rewrite exists to end.

       THE PANEL-BLINDNESS INCIDENT IS WHY THIS FILE CLICKS PANELS, and it is
       kept here because it is the same failure one layer down -- a corpus
       that cannot see what it claims to count. a5c9399 put 84 new title=
       attributes on the poster cards, 123 to 207 across the five screen
       files, and this source did not move: the same figure on the tree before
       that commit and on the tree after it, both measured by running each
       tree's own copy of this file. A corpus that cannot see 84 new instances
       of the exact attribute it counts is not measuring the page. The comment
       that stood here at the time explained the small number as a <td>
       exclusion working as designed. It was not. It was two whole panels the
       sweep had never opened. */
    'title': 1050,
    /* Regression: .topbar leaves the scope list. Cost 1 x 110 = 110.
       Slack 43. UNCHANGED DELIBERATELY -- it already clears the criterion
       with room, and the criterion is a maximum on slack, not a target to
       raise a floor toward. */
    'placeholder': 245,
    /* Regression: .topbar leaves the scope list. Cost 5 x 110 = 550.
       Slack 218. UNCHANGED DELIBERATELY, for the same reason as placeholder:
       comfortably under its ceiling, and nothing is bought by moving it up. */
    'option': 1400,
    /* Not a rendered source at all -- see the §17 block below for why it is
       here -- so the chrome-element regression does not apply and this floor
       needs a ceiling of its own.

       THE REGRESSION IS THE SLICE. §17's copy is read out of one span of
       ARCHITECTURE.md, and the way that quietly shrinks is the span ending
       early or a subsection dropping out of it, which costs whatever that
       subsection contributes.

       MEASURED 2026-08-22, by attributing every match to the subsection
       heading it falls under. Nine subsections contribute (17.1 and 17.6
       contribute none): 17.8 is the largest, then 17.5, 17.2, 17.3, and
       17.3.3 and 17.4 tie in the middle, with 17.3.2, 17.7 and finally 17.3.1
       smallest at a single string.

       THE SMALLEST IS NOT USABLE AS THE CEILING. 17.3.1 contributes 1, which
       would force slack to 0 and make this floor fire on any edit that
       removed a single quoted label anywhere in §17 -- a section under active
       authorship. THE MEDIAN SUBSECTION IS USED INSTEAD, and it contributes
       7. That is the honest reading of "losing a subsection": the typical
       one, not the degenerate one.

       Cost 7. Slack 3, under it.

       WAS 45, AND THE TRUE RELATION IS SMALLER THAN THIS COMMENT USED TO
       CLAIM. Measured against the population of 2026-08-22, 45 carried 33 of
       slack. That is 1.3x the largest subsection -- 17.8, at 25 -- and a
       little under five times the median of 7. Not "several times the
       largest". What was and remains true is the part that decided it: 33 is
       OVER 25, so 45 sat green through the loss of any single subsection in
       the section, the largest one included. */
    'ARCHITECTURE §17 copy': 75,
  };
  const seenBySource = {};
  const countSource = (src, n) => { seenBySource[src] = (seenBySource[src] || 0) + n; };

  /* ---------------------------------------------------------------------
   * THE EXCLUSION'S OWN CEILING, and the reason it is a gate and not a print.
   *
   * `data-copy="data"` takes strings out of this corpus. Until 2026-08-22 the
   * only thing standing behind that was the word COUNTED: the skip total is
   * printed beside the corpus total on the pass line, and printing is not
   * gating. A widened exclusion is the one regression in this rule the
   * per-source floors above structurally cannot reach, because it is
   * ELEMENT-granular -- the cost is the marked element's strings times only
   * the combinations that element renders in, which is 8 on Libraries and
   * fewer on a single state. Counting the OPT-OUTS instead of the strings they
   * removed is what closes the class at every granularity: one element is one
   * marking whatever it costs.
   *
   * THE ARITHMETIC. The tree carries 5 rendered opt-outs (measured
   * 2026-08-22; the pass line prints the live figure on every run, and this is
   * a ceiling derived from that baseline rather than a second copy of it). The
   * cheapest widening worth catching is ONE further element on the SMALLEST
   * screen -- Libraries, 8 of the 110 combinations -- which puts the total on
   * 13. The ceiling is therefore the largest value that still FAILS on 13,
   * which is 12.
   *
   * FIRED THREE WAYS, 2026-08-22, rather than argued. One
   * `#pg-libraries [placeholder]` element marked `data-copy="data"` put the
   * total on exactly 13 -- red here, and green on every other line in the run,
   * including the `placeholder` floor it cost 8 of. One
   * `.topbar button[aria-label]` marked the same way put it on 115 -- red
   * here, where before this ceiling existed that drill dropped 110 aria-label
   * strings and the whole gate still exited 0. The untouched tree reads 5 and
   * is green.
   *
   * IT IS A CEILING, SO A LEGITIMATE NEW OPT-OUT TURNS IT RED, and that is the
   * intent: widening this exclusion should be something a person argues for,
   * not something that lands. The response is the one the floor block above
   * states -- re-derive, take the smallest value that satisfies the
   * derivation, and name the new element here.
   * ------------------------------------------------------------------- */
  const OPTOUT_CEILING = 12;

  let strings = 0, exempted = 0, glyphs = 0, dataOptouts = 0; const bad = [];
  /* §17's copy is counted apart from `strings` on purpose. STRING_FLOOR's
     margin below is DERIVED from the rendered corpus and argued against a
     293-string regression — the working, and the single dated figure it rests
     on, are under STRING_FLOOR itself rather than restated a second time here
     — and folding documentation strings into that total would move the number
     the derivation is about while claiming the derivation still held. */
  let s17Strings = 0, s17Exempted = 0, s17Bad = 0, s17Glyphs = 0;
  /* Four floors, each against the corpus MEASURED WHEN IT WAS SET, which is the
     only tense any of these numbers can be read in: 2000 against 4203 while no
     panel was ever opened; 5800 against 6685 once the traversal landed; 6750
     against 6978 when the unit became a run of inline content rather than a
     childless block element; and 13200 now that a <td> is in the corpus like
     anything else.

     THE MARGIN IS DERIVED, NOT ROUNDED, and the criterion is this file's own:
     restoring the pre-PG-03 element-based unit costs 293 strings, so a floor
     with more than 293 of SLACK — corpus minus floor — would sit green through
     exactly the regression it was moved for. The working, measured 2026-08-22
     on the tree that put cells into the corpus:

         13,411   strings read, both installs, every state, every panel pass
       − 13,200   floor
       ────────
            211   slack, against a ceiling of 293, and 82 under it

     SLACK IS DELETION TOLERANCE. The floor fires when strings go missing, so
     211 is how many can be removed before it does — it is not a budget for
     additions, which move the count the other way, and one authored string is
     counted once per combination it renders in.

     293 is carried unchanged from the pre-cells corpus rather than rescaled to
     the new one. The same regression over a corpus twice the size would cost
     MORE than 293, so holding the ceiling where it is reads the criterion
     strictly rather than staling it.

     THE 13,411 IS DATED, NOT LIVE, and it is the only figure here that is a
     measurement at all. The pass line prints the real count on every run, so
     restating it in this comment would be a second copy with no way to stay
     true — which is what happened to the last one: it read "6978" against a
     tree that had drifted to 6908 by the time anyone counted again. Both
     earlier margins also failed the criterion outright: 5800 against 6685 had
     885 of slack, enough to lose every group heading twice over. */
  const STRING_FLOOR = 13200;

  /* ---------------------------------------------------------------------
   * §17's own shipping copy, and the laundering channel that was in this rule.
   *
   * Everything else here reads the MOCKUP. But a UI label is authored in
   * ARCHITECTURE §17 first and copied into a mockup afterwards, and §17
   * appeared in this file only as the SOURCE OF AN EXEMPTION: exempt() blesses
   * a short em-dash string when §17 contains its two-words-either-side window.
   * So the order of operations laundered the rule. Write the em dash into §17,
   * copy the label into a mockup, and the mockup string is exempt BECAUSE §17
   * says so — which means a §17-originated em dash could never be caught here,
   * by construction, no matter how many mockups repeated it. One was drafted
   * during the §17.8 pass and removed by hand rather than by this file.
   *
   * ⚠️ THE HOLE IS THE CORPUS, NOT THE FIFTEEN-WORD FLOOR, and the floor is
   * easy to read backwards. `< 15` is what FIRES: the floor does not exempt
   * short strings, it confines the rule TO them. That is §13's actual position
   * — "the em dash is fine in prose and banned in UI microcopy", on the ground
   * that "a sentence long enough to need one is already too long for a button,
   * a tooltip, a toast or an empty state" — so raising or removing it would
   * fire on every paragraph of legitimate prose in the product. It is left
   * exactly as it was, and §17's copy is brought INTO the corpus instead.
   *
   * Shipping copy in §17 is marked `*"…"*`, the italic-quoted form every
   * specified label and sentence uses; 57 spans today. They run through the
   * same checkCopy, and the §17 exemption is withheld from them, because a
   * string cannot be its own authority. exempt() itself is untouched: with the
   * source now gated, the mockup exemption can only propagate copy that has
   * already been through the rule, which is the layering it always implied.
   * ------------------------------------------------------------------- */
  const S17_EMDASH_ALLOWED = new Map([
    /* All four predate this sweep and are the head-and-detail construction
     * DESIGN-DIRECTION prescribes — §1.4's "UI copy states facts about system
     * state", whose worked example this file used to cite as §13 endorsing the
     * EM DASH: it read "Sonarr unreachable — connection refused at
     * 10.0.0.4:8989" and was the copy to imitate, two sections from §13's ban
     * on that punctuation in the same string.
     *
     * ⚠️ RULED 2026-08-17, and the four entries below are unchanged by it. The
     * construction stays prescribed; its beat is now a COLON for a statement
     * and its reason ("Sonarr unreachable: connection refused at …"), or a full
     * stop and two sentences for a statement and an instruction. §1.4 and §13's
     * Copy block both say so at the endorsement, so an author who never reaches
     * the ban still lands on the right glyph. That makes these four §17's
     * EXISTING wording rather than a licence for new copy — they are carried,
     * not blessed, because §17's shipping copy is the owner's to word and a
     * checker does not edit the specification it checks. A fifth is a finding.
     * The fifteen-word
     * proxy cannot see that construction, so each instance is recorded here
     * rather than generalised into a grammar rule this file would then have to
     * be right about — an enumerated four fails on the fifth and asks a human,
     * where a shape rule would silently bless everything shaped like it.
     * Matched after norm(), so a rewrite loses the exemption and a change of
     * emphasis does not: the same property exempt() has. */
    /* ⚠️ Re-keyed from Kavita to Komga on 2026-08-17, tracking a ruling made in
     * §17 rather than here: §17.7's degraded banner may not use Kavita as the
     * exemplar for "no delta channel at all", because ADR-0035 §2a verified
     * against a live instance that Kavita HAS a delta (channel 3b's page walk)
     * and lacks only a changed-since endpoint. The construction this entry
     * exempts is unchanged and so is its RETIRED BY; only the subject moved,
     * and it moved because the size check below fails on an exemption that
     * matches nothing — which is exactly how this file learns that §17 ruled. */
    /* ⚠️ Re-keyed again on 2026-08-21, appending `, 3 hours ago`, and tracking a
     * ruling made in §17 for the second time: 70192dcd2bc9 brought both of
     * §17.7's exemplars into the absolute-plus-relative form DESIGN-DIRECTION
     * §9.1 requires — the section that names THIS banner as its worked failure,
     * because "showing cached data from 11:47" reads the same at six minutes and
     * at twenty-two hours. So the old key encoded the violation: it was the
     * pre-ruling wording, and matching it was this file vouching for a sentence
     * §9.1 bans.
     *
     * The red that resulted was the mechanism REPORTING A REAL DESYNC, not a
     * broken check. §17 moved, the registered answer had not caught up, and
     * exact-match keying after norm() is what made the gap visible at all. Do
     * not soften it into a prefix, a fuzzy compare, or a norm() that strips the
     * relative clause — a tolerant key would have matched both wordings and
     * reported nothing, which is the failure mode this map exists to avoid.
     * Note also which line fired: the §17 violation above short-circuits the
     * size check below, so a stale key surfaces as a VIOLATION and never as
     * "matched nothing". The two guards are not interchangeable. */
    ['komga is unreachable — showing cached data from the last full compare at 09:12, 3 hours ago',
      'The stale-data banner exempt() was built for, at §17 source. Head is the ' +
      'failing component, detail is the observed symptom and its time. ' +
      'RETIRED BY: §13 gaining a construction rule the checker can evaluate.'],
    ['grab failed — http 502',
      'Error copy in the specified form: failing verb, then the upstream status ' +
      'verbatim, which §17.6 requires be shown rather than summarised. ' +
      'RETIRED BY: the same.'],
    ['tv — catalogue source, request destination',
      "§17.8's per-kind source gloss. The head is a media kind and the detail is " +
      'what the kind gets, which is a label-and-value pair rather than a sentence. ' +
      'RETIRED BY: the same, or by the gloss becoming a real two-column row.'],
    ['music — catalogue source; no request destination',
      'The negative half of the pair above, and it must keep the same shape as ' +
      'its positive or the two stop reading as one column. RETIRED BY: the same.'],
    /* ✅ The two entries that used to sit here are RETIRED, both by the ruling
     * their own RETIRED BY clause asked for, and both in §17 rather than here —
     * which is the direction that was always right: this file raised them and
     * did not rewrite them, because a checker that edits the specification it
     * checks is not a checker.
     *
     *   §17.5, "… Dune (2021). — [Show it]"  ruled NOTATION, not one string. The
     *     em dash was §17's join between a sentence and a control and the
     *     brackets were the control. §17 now writes them as what they are: two
     *     `*"…"*` spans, one per element the user sees, so there is no em dash
     *     to exempt and no ambiguity for the next reader either.
     *   §17.8, "… can only offer to add sources — never reshape it"  ruled a
     *     REAL finding, as this file suspected. Rewritten in §17 as three plain
     *     sentences, same meaning, no mid-sentence beat.
     *
     * Both are gone from this map rather than left as passing entries, because
     * the size check below fails on an exemption that matches nothing — which is
     * the mechanism that makes this retirement provable rather than asserted. */
  ]);

  /* One place the three §13 rules are applied, so the rendered walk, the
     attribute sweep and the §17 sweep below cannot drift into checking
     different things. `isS17` moves the BOOKKEEPING — which counter, and which
     exemption — and never a rule: all three rule bodies are literally shared,
     which is the property this single function exists to hold. */
  const checkCopy = (where, t, isS17 = false) => {
    if (isS17) s17Strings++; else strings++;
    const low = t.toLowerCase();
    for (const w of BANNED_WORDS) {
      if (new RegExp('\\b' + w.replace('-', '[- ]') + '\\b').test(low)) {
        bad.push(`${where}: banned word "${w}" in "${t.slice(0, 70)}"`);
      }
    }
    if (t.includes('!')) bad.push(`${where}: "!" in "${t.slice(0, 70)}"`);
    /* STRUCTURAL EXEMPTION: A BARE EM DASH IS A GLYPH, NOT A SENTENCE.
       `—` alone, as the WHOLE trimmed string, is the typographic convention for
       "no value here" — the shipped app's NOTHING.empty, and the mockups' own
       cell filler. §13's em-dash rule is a rule about PROSE: it bans a beat
       inside a sentence, and there is no sentence here to carry a beat.

       It is exempted BY SHAPE, never by name, file or token, which is what
       makes it unable to launder anything: the test is that the string is one
       character long, and no sentence fits in one character. Contrast the
       exemption above it, which needs §17 to have written the wording — that
       one is a claim about a document and can go stale; this one is a claim
       about the string itself and cannot. It applies to both corpora for the
       same reason: a glyph in §17 is a glyph too, and withholding it there
       would be withholding it from a rule that was never about §17.

       Deliberately NOT floored. Every count in this file has a floor because a
       check that matches nothing reads like a check that passed — but that
       argument is about RULES, not about exemptions of shape. The number is
       printed regardless, which is what makes it readable: it was zero for as
       long as `<td>` was excluded wholesale, because a bare dash is the mockups'
       own cell filler and every one of them was in a cell. Cells are in the
       corpus now, so it counts them. */
    if (t.trim() === '—') { if (isS17) s17Glyphs++; else glyphs++; return; }
    /* ⚠️ The fifteen-word floor is a PROXY for "is this a UI string", and it
       earns its keep only where the corpus MIXES microcopy with prose — the
       rendered walk reads paragraphs as well as buttons, and firing on those
       is the false positive the floor exists to prevent. §17's `*"…"*` spans
       are specified UI strings by construction, so there the proxy has no
       question left to answer and is not applied. That is the floor's own
       stated reason read literally: §13 grounds it on "a sentence long enough
       to need one is already too long for a button, a tooltip, a toast or an
       empty state", which is a claim about the ELEMENT, not about the count.
       Narrowing it to the corpus it was meant for is what catches the 24-word
       §17.8 sentence that was drafted with an em dash and fixed by hand — the
       instance this sweep was written for, and one a floor of any length would
       have missed. */
    if (t.includes('—') && (isS17 || t.split(/\s+/).length < 15)) {
      /* A §17 string cannot be exempted BY §17 — that is the whole hole this
         sweep closes, so it is checked against the recorded set instead. */
      if (isS17 ? S17_EMDASH_ALLOWED.has(norm(t)) : exempt(t)) { if (isS17) s17Exempted++; else exempted++; }
      else bad.push(`${where}: em dash in ${isS17 ? 'specified UI copy' : 'a string under 15 words'} — "${t.slice(0, isS17 ? 140 : 70)}"`);
    }
  };

  /* Read once: it is a property of the document, not of a screen or a state,
     and reading it inside the loop would multiply one string by 74. */
  {
    const t = await page.evaluate(() => document.title.replace(/\s+/g, ' ').trim());
    countSource('document.title', t ? 1 : 0);
    if (t) checkCopy('document.title', t);
  }

  /* §17's shipping copy, read once for the same reason: it is a property of the
     document, not of a screen or a state. The span is `*"…"*` and the bold form
     `**"…"**` falls out of the same pattern, which is wanted — the emphasis a
     sentence is quoted with is not what makes it a label. */
  {
    for (const m of s17Body.matchAll(/\*"([^"]+)"\*/g)) {
      const t = m[1].replace(/\s+/g, ' ').trim();
      if (!t) continue;
      countSource('ARCHITECTURE §17 copy', 1);
      const before = bad.length;
      checkCopy('ARCHITECTURE §17', t, true);
      s17Bad += bad.length - before;
    }
  }

  for (const install of INSTALLS) {
  await setInstall(page, install);
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    const states = await statesOf(page, screen);
    for (const state of states) {
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(20);
      for (const pass of await panelsOf(page, screen)) {
      await setPanels(page, screen, pass);
      const where = `${install}/${screen}/${state}${panelLabel(pass)}`;
      /* The invisible-to-layout half of the corpus, gathered under exactly the
       * structural exclusions the rendered walk uses. `<option>` is collected
       * from a closed menu on purpose: that is the only state it is ever in
       * during this sweep, and it is why the walk could not see it. */
      const attrsRead = await page.evaluate(({ s, ATTRS }) => {
        const out = [];
        let skipped = 0;
        const root = document.querySelector('#pg-' + s);
        const chrome = [root, document.querySelector('.topbar'), document.querySelector('.sidebar')]
          .filter(Boolean);
        for (const scope of chrome) {
          scope.querySelectorAll('*').forEach((el) => {
            if (el.closest('[hidden]')) return;
            if (el.closest('.statebar')) return;     /* the mockup's own scaffolding */
            /* The declared opt-out, read once per element and applied to every
               string the element contributes. Counted, never silent. */
            const isData = !!el.closest('[data-copy="data"]');
            for (const a of ATTRS) {
              const v = el.getAttribute(a);
              if (v == null) continue;
              const t = v.replace(/\s+/g, ' ').trim();
              if (!t) continue;
              if (isData) skipped++; else out.push({ src: a, text: t });
            }
            if (el.tagName === 'OPTION' || el.tagName === 'OPTGROUP') {
              const t = (el.tagName === 'OPTION' ? el.textContent : el.label || '')
                .replace(/\s+/g, ' ').trim();
              if (t) { if (isData) skipped++; else out.push({ src: 'option', text: t }); }
            }
          });
        }
        return { attrs: out, skipped };
      }, { s: screen, ATTRS });
      dataOptouts += attrsRead.skipped;
      for (const a of attrsRead.attrs) {
        countSource(a.src, 1);
        checkCopy(`${where} [${a.src}]`, a.text);
      }

      const r = await page.evaluate((s) => {
        /* The unit is a STRING, not a text node. A sentence interrupted by an
         * inline <a> or <code> is one string that happens to be three nodes,
         * and splitting on nodes turns "…retried automatically — and the row
         * says so" into a two-word fragment carrying an em dash. That much is
         * unchanged. What changed is HOW the unit is found.
         *
         * IT USED TO BE AN ELEMENT: "the innermost element that lays out as a
         * block with no blockish children". That definition drops the element's
         * OWN direct text on the floor the moment it acquires one blockish
         * child, and CSS hands out blockish children without being asked —
         * every child of a flex or grid container is blockified, whatever its
         * own `display` says. `.section__head h2` is `display: flex`, so
         * `<h2>Ebooks <span class="section__count">14</span></h2>` had a
         * blockish child, failed the second test, and contributed nothing;
         * `.section__count` passed the test on its own and contributed "14".
         * The word "Ebooks" — a group heading, on the two screens that group —
         * was never read. Neither was `.card__meta`'s media-type word, nor
         * `.avail`'s "10/10", nor `.detail li`'s prose, nor `.banner__text`,
         * nor eleven `.pagehead__meta` summaries: 59 distinct strings, all of
         * them the product's own voice, all of them invisible to §13 since 1b
         * was written. Logged as PG-03.
         *
         * IT IS NOW A RUN OF INLINE CONTENT INSIDE ONE BLOCK BOX, which is what
         * the browser itself lays out: walk the text nodes in document order,
         * attribute each to its nearest blockish ancestor, and cut the string
         * wherever that ancestor changes. Three consequences, all wanted:
         *
         *   · An element's own text is never lost, because every text node is
         *     attributed to something. This is the fix.
         *   · An inline <a> or <code> does NOT cut the run — it is not blockish,
         *     so its text keeps its parent's block as its ancestor. The reason
         *     the old comment gives for the unit's shape still holds exactly.
         *   · A BLOCKIFIED child DOES cut the run, and should: `.detail li` is
         *     `display: flex` with a `gap`, so "Recomputed <span>13:40</span>,
         *     28 minutes ago" is three gapped boxes on the screen, not one
         *     sentence. Cutting there splits some authored sentences into
         *     fragments, which makes the em-dash-under-15-words rule STRICTER
         *     rather than laxer — a five-word box carrying an em dash is the
         *     thing §13 bans, and the fragment is what the reader sees.
         *
         * And one correctness win that falls out rather than being aimed at:
         * `textContent` reads through `[hidden]` descendants, so the old walk
         * concatenated both `[data-inst]` variants of a summary line into
         * strings no user ever sees — "No results for duen in 82 libraries."
         * was the 8 and the 2 of two hidden alternatives run together. A text
         * walk tests each node's own ancestry, so the variants stay apart. */
        const out = [];
        let skipped = 0;
        const root = document.querySelector('#pg-' + s);
        const BLOCKISH = /^(block|flex|grid|list-item|table-caption)$/;
        const blockOf = (n) => {
          for (let e = n.parentElement; e; e = e.parentElement) {
            if (BLOCKISH.test(getComputedStyle(e).display)) return e;
          }
          return null;
        };
        const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
        let cur = null, run = '', isData = false;
        /* One buffer, two destinations. A run that lies inside a declared
           `data-copy="data"` is COUNTED rather than checked, in exactly the unit
           the corpus itself uses — a run of inline content, not a text node — so
           the printed opt-out figure and the printed corpus figure are the same
           kind of number and can be read against each other. */
        const flush = () => {
          const t = run.replace(/\s+/g, ' ').trim();
          if (t) { if (isData) skipped++; else out.push(t); }
          run = '';
        };
        for (let n = w.nextNode(); n; n = w.nextNode()) {
          const p = n.parentElement;
          if (!p) continue;
          /* The SAME structural exclusions as before, applied to the text
             node's own ancestry. `closest` reaches through inline wrappers, so a
             word inside `<td><b>…</b></td>` is judged by the cell's own
             declaration and not by the <b>. */
          if (p.closest('[hidden]')) continue;
          if (p.closest('.statebar')) continue;       /* the mockup's own scaffolding */
          /* CUT THE RUN AT A DECORATIVE DUPLICATE, and do not merely drop it.
             The responsive table layout puts `<span class="stacklabel"
             aria-hidden="true">Publisher</span>` in front of the cell's value; at
             1440 that span is `display: none`, but the walk attributes a text
             node to its nearest BLOCKISH ancestor — the cell — and tests that
             ancestor's visibility, never the node's own inline chain, so the
             hidden label was read anyway and welded to what follows it. That is
             where "PublisherBOOM! Studios", "Items—" and "Sent to—" came from:
             99 of the 140 violations this rule reported the moment cells entered
             the corpus were a label and a value run together, and not one of
             them was a string any user or any screen reader ever gets. The label
             is already read once from the <th> it duplicates. flush() rather than
             `continue` so the sides do not weld to each other either. */
          if (p.closest('[aria-hidden="true"]')) { flush(); continue; }
          /* The declared opt-out. It cuts the run for the same reason: a
             substituted value sitting mid-sentence must not take the authored
             words around it out of the corpus with it. */
          const nowData = !!p.closest('[data-copy="data"]');
          const b = blockOf(n);
          if (!b || (!b.offsetParent && b.tagName !== 'BODY')) continue;
          /* Whitespace-only nodes are the spaces BETWEEN inline elements. They
             carry no string of their own, but dropping them would weld "and"
             to "<a>the row</a>". They join the current run and are trimmed off
             at the ends by flush(). */
          if (b !== cur || nowData !== isData) { flush(); cur = b; isData = nowData; }
          run += n.nodeValue;
        }
        flush();
        return { text: out, skipped };
      }, screen);
      dataOptouts += r.skipped;
      for (const t of r.text) checkCopy(where, t);
      }
    }
    await page.selectOption('#pg-' + screen + ' [data-act="state"]', states[0]);
    await resetPanels(page, screen);
  }
  }
  await setInstall(page, 'full');
  const uniq = [...new Set(bad)];
  if (uniq.length) {
    fail(`§13 copy: ${uniq.length} violation(s) in user-visible text`);
    /* THE REMEDY TRAVELS WITH THE FAILURE. This rule reads cells as well as
       chrome, so the author it fires on is often somebody who has just typed a
       sample value into a mockup and has never read a line of this file. A
       finding they cannot act on without institutional memory is a finding that
       gets worked around. Printed above the violations, not below them: ten
       notes is exactly enough to bury it. */
    note('remedy: rewrite the string if UsArr wrote it — drop the em dash rather than padding the sentence past fifteen words, and pick a plainer word than the banned one.');
    note('remedy: if it is a SUBSTITUTED VALUE and not copy — a publisher, a release name, a title from a database — put data-copy="data" on the element that holds it, and only that element. Text and attributes inside it leave the corpus.');
    note(`remedy: that opt-out is counted and capped, not silent — every skip is printed beside the corpus total on the pass line (${dataOptouts} skipped on this run, ceiling ${OPTOUT_CEILING}), so it is a thing this check has seen and dismissed rather than a thing it never found, and widening the exclusion far enough fails the run on its own line.`);
    uniq.slice(0, 10).forEach((b) => note(b));
  }
  else if (floorOk('§13 copy', strings, STRING_FLOOR, 'user-visible string(s)')) {
    ok(`§13 copy: ${strings} user-visible strings clean of banned words, "!" and short-string em dashes ` +
      `(floor ${STRING_FLOOR} over both installs; ${exempted} short em-dash string(s) exempt because ARCHITECTURE §17 fixes their wording verbatim; ` +
      `${glyphs} bare "—" string(s) read as a glyph rather than prose, a structural exemption no sentence can hide in; ` +
      `${dataOptouts} string(s) skipped at a declared data-copy="data", counted here rather than passed over in silence)`);
  }
  /* GATED, NOT MERELY PRINTED -- see OPTOUT_CEILING above for the derivation
     and the three drills. Deliberately outside the branch above: a widened
     exclusion must be reported whether or not the copy sweep itself found a
     violation, and the `fail` path returns early past the pass line. */
  if (dataOptouts > OPTOUT_CEILING) {
    fail(`§13 copy opt-outs: ${dataOptouts} string(s) skipped at a declared data-copy="data", over the ceiling of ${OPTOUT_CEILING} — ` +
      `the exclusion widened. Either an element was marked data-copy="data" that was not before, or one already marked now covers more. ` +
      `This is counted on the OPT-OUTS and not on the strings they removed, because the cost of one marking is only the combinations that ` +
      `one element renders in — 8 on Libraries — which is under every per-source floor in this rule and always will be.`);
  } else {
    ok(`§13 copy opt-outs: ${dataOptouts} string(s) skipped at a declared data-copy="data" (ceiling ${OPTOUT_CEILING}), ` +
      `so the exclusion is capped and not only counted`);
  }
  /* Reported separately from the line above because it is a separate corpus
     with a separate exemption, and one combined number would hide which of the
     two moved. Printing the recorded count is the point: it is the number that
     goes up when a fifth short em dash is argued into §17's copy. */
  if (s17Bad) {
    /* The violations themselves are already in `bad` and printed above; this
       line exists so the §17 sweep never prints "clean" beside its own
       failure, which is the shape of pass this file was rewritten to stop. */
    fail(`§13 copy §17: ${s17Bad} violation(s) in ARCHITECTURE §17's own shipping copy, out of ${s17Strings} string(s) read`);
  } else if (S17_EMDASH_ALLOWED.size !== s17Exempted) {
    fail(`§13 copy §17: ${S17_EMDASH_ALLOWED.size} recorded em-dash exemption(s) but ${s17Exempted} matched — ` +
      `a recorded string no longer appears in §17, so the record is describing copy that is not there`);
  } else {
    ok(`§13 copy §17: ${s17Strings} shipping-copy string(s) in ARCHITECTURE §17 clean of banned words, "!" and ` +
      `em dashes at ANY length (the fifteen-word floor is not applied to specified UI copy; ${s17Exempted} ` +
      `recorded exception(s) and ${s17Glyphs} bare "—" glyph(s), the exception(s) all §17's existing wording of the head-and-detail construction ` +
      `§1.4 prescribes — whose beat is a colon since 2026-08-17, so these are carried, not blessed; ` +
      `§17 cannot exempt itself here, which is the point)`);
  }
  /* Each non-layout source is floored on its own. A source that stops being
   * collected -- an attribute renamed, a selector narrowed, a <title> dropped
   * by the generator -- fails here instead of quietly contributing nothing. */
  for (const [src, floor] of Object.entries(CORPUS_FLOORS)) {
    if (floorOk(`§13 copy corpus ${src}`, seenBySource[src] || 0, floor, 'string(s) read')) {
      ok(`§13 copy corpus: ${seenBySource[src]} ${src} string(s) read (floor ${floor}), a source the rendered walk cannot see`);
    }
  }
}

await browser.close();
console.log(failures ? `\n${failures} FAILURES` : '\nall design checks pass');
process.exit(failures ? 1 : 0);
