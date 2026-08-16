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
 *   4  overflow                         — 5 widths x 5 screens x every state
 *   5  row heights                      — against the three density bands
 *   6  availability accessible names    — no silent ✓/✗
 *   7  one tab stop per list            — roving tabindex, or a declared opt-out
 *   8  containment is live              — the content-visibility-on-<tr> guard
 *   9  webfont                          — IBM Plex actually resolves
 * ============================================================================= */

import { chromium } from '/opt/node22/lib/node_modules/playwright/index.mjs';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, relative } from 'node:path';

const DESIGN = dirname(fileURLToPath(import.meta.url));
const MOCKUPS = join(DESIGN, 'mockups');
const ROOT = join(DESIGN, '..', '..');
const URL = 'file://' + join(MOCKUPS, 'prototype.html');

const SCREENS = ['home', 'services', 'libraries', 'search', 'requests'];
const WIDTHS = [390, 1280, 1440, 1680, 1920];
const DENSITIES = ['compact', 'standard', 'relaxed'];

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
const SOURCES = [
  join(DESIGN, 'tokens.css'),
  join(MOCKUPS, 'usarr.css'),
  join(MOCKUPS, 'fonts.css'),
  join(MOCKUPS, 'usarr.js'),
  ...readdirSync(MOCKUPS).filter((f) => f.endsWith('.html') && f !== 'prototype.html')
    .sort().map((f) => join(MOCKUPS, f)),
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

/* Scan every stripped source for a regex. Returns [{file, line, text}]. */
function scan(re, filter = () => true) {
  const hits = [];
  for (const f of FILES) {
    if (!filter(f)) continue;
    const r = new RegExp(re.source, re.flags.includes('g') ? re.flags : re.flags + 'g');
    let m;
    while ((m = r.exec(f.src)) !== null) {
      hits.push({ file: rel(f.path), line: lineOf(f.src, m.index), text: m[0].trim().slice(0, 90) });
      if (m.index === r.lastIndex) r.lastIndex++;
    }
  }
  return hits;
}

/* A named static rule: fails with every hit listed, passes with a count. */
function rule(name, re, filter = () => true, allow = () => false) {
  const hits = scan(re, filter).filter((h) => !allow(h));
  if (hits.length) {
    fail(`${name} — ${hits.length} hit(s)`);
    for (const h of hits.slice(0, 8)) note(`${h.file}:${h.line}  ${h.text}`);
    if (hits.length > 8) note(`… and ${hits.length - 8} more`);
  } else {
    ok(name);
  }
  return hits.length;
}

/* =============================================================================
 * 1. §13 ban list
 * ========================================================================== */
head('1. DESIGN-DIRECTION §13 ban list (static, comments stripped)');
note(`${FILES.length} source files: ` + FILES.map((f) => rel(f.path).replace('docs/design/', '')).join(' '));

/* --- colour ------------------------------------------------------------- */
rule('§13 colour: no indigo/violet/purple/fuchsia', /\b(indigo|violet|purple|fuchsia)\b/i);
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
  if (bad.length) { fail('§13 type: banned family in a font stack'); bad.forEach((b) => note(b)); }
  else ok(`§13 type: no banned family in any font stack (${stacks} font-family declarations parsed, families matched whole)`);
}
banFonts();

rule('§13 type: no Google Fonts reference', /fonts\.(googleapis|gstatic)\.com/i);
rule('§13 type: no --text-empty / --fs-empty token', /--(text|fs)-empty\b/);
rule('§13 type: no font-size token above 20px', /--(?:text|fs)-[a-z0-9]+\s*:\s*(2[1-9]|[3-9]\d|\d{3,})px/);
rule('§13 type: no uppercase transform on a label, no italic heading',
  /text-transform\s*:\s*uppercase|h[1-6][^{}]*\{[^}]*font-style\s*:\s*italic/i);
/* §9.6: centred text is legal in a dialog and nowhere else. */
rule('§13 type: no text-align:center outside dialog', /text-align\s*:\s*center/i,
  () => true,
  (h) => /dialog|modal|toast/i.test(h.text));
rule('§13 type: no bordered / filled empty state',
  /\.empty[a-z-]*[^{}]*\{[^}]*(border\s*:|border-style\s*:\s*dashed|background\s*:|box-shadow\s*:)/i);

/* --- layout ------------------------------------------------------------- */
rule('§13 layout: no backdrop-filter', /backdrop-(filter|blur)/i);
rule('§13 layout: no arbitrary-value class syntax', /class="[^"]*\[[^\]]*(px|rem|#[0-9a-f]{3})[^\]]*\]/i);
(function radius() {
  const hits = scan(/border-radius\s*:\s*([0-9.]+)px/gi);
  const over = hits.filter((h) => parseFloat(h.text.match(/([0-9.]+)px/)[1]) > 6);
  if (over.length) { fail('§13 layout: border-radius above the 6px ceiling'); over.forEach((h) => note(`${h.file}:${h.line}  ${h.text}`)); }
  else ok(`§13 layout: every literal border-radius ≤ 6px (${hits.length} literal radii, plus token references)`);
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
  if (bad.length) { fail('§13 icons: banned or unlisted icon'); bad.forEach((b) => note(b)); }
  else ok(`§13 icons: ${ids.size} icon ids all on the allowlist, ${imports} import specifier(s) clean ` +
    `(bans evaluated on import specifiers and <symbol id> only, so a KeyboardEvent.key never enters)`);
}
banIcons();

rule('§13 icons: no emoji codepoints in app source', /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{FE0F}]/u,
  (f) => f.kind !== 'html' /* the mockup's own ⚠/✓ are SVG symbols; prose files are docs */);

/* --- motion ------------------------------------------------------------- */
(function motion() {
  const durs = scan(/(?:transition|animation)(?:-duration)?\s*:\s*[^;{}]*?([0-9.]+)m?s/gi);
  const over = durs.filter((h) => {
    const m = h.text.match(/([0-9.]+)(ms|s)\b/);
    if (!m) return false;
    return (m[2] === 's' ? parseFloat(m[1]) * 1000 : parseFloat(m[1])) > 200;
  });
  if (over.length) { fail('§13 motion: transition-duration above 200ms'); over.forEach((h) => note(`${h.file}:${h.line}  ${h.text}`)); }
  else ok(`§13 motion: ${durs.length} literal duration(s), none above 200ms`);

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
(function skipLink() {
  const pages = FILES.filter((f) => f.kind === 'html');
  const missing = pages.filter((f) => !/skip[^<]{0,20}to[^<]{0,20}content/i.test(f.raw));
  if (missing.length) { fail('§13 controls: no "Skip to content" link'); missing.forEach((f) => note(rel(f.path))); }
  else ok(`§13 controls: every one of the ${pages.length} screen files carries a Skip to content link`);
})();

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
const browser = await chromium.launch();

async function statesOf(page, screen) {
  return page.$$eval('#pg-' + screen + ' [data-act="state"] option', (os) => os.map((o) => o.value));
}

/* --- 4. overflow --------------------------------------------------------- */
head('4. Overflow: nothing past the viewport, nothing past its own .tablewrap');
for (const width of WIDTHS) {
  const ctx = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await ctx.newPage();
  await page.goto(URL);
  let worstOver = 0, worstWhere = '', combos = 0;
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(60);
    for (const state of await statesOf(page, screen)) {
      combos++;
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(40);
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
      if (r.clip.length) fail(`${width}px ${screen}/${state}: sheared by overflow-x:clip on .tablewrap\n        ` + r.clip.join('\n        '));
      if (r.over) {
        worstOver = Math.max(worstOver, r.worst - width);
        worstWhere = `${width}px ${screen}/${state}: ${r.over} over, worst right=${r.worst}\n        ${r.ex.join('\n        ')}`;
      }
      if (r.doc > width) fail(`${width}px ${screen}/${state}: document scrolls sideways (${r.doc})`);
    }
  }
  if (worstOver > 0) fail('overflow ' + worstWhere);
  else ok(`overflow at ${width}px: ${combos} screen×state combinations, nothing past the viewport or its wrapper`);
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
for (const density of DENSITIES) {
  await page.evaluate((d) => { document.documentElement.setAttribute('data-density', d); }, density);
  const rowH = await page.evaluate(() => parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--row-h')));
  const shift = rowH - 28;
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    const states = await statesOf(page, screen);
    const hs = [];
    for (const state of states) {
      if (state === 'annex') continue; /* labelled as documentation for services v0.1 lacks */
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(30);
      hs.push(...await page.evaluate((s) => [...document.querySelectorAll('#pg-' + s + ' tbody tr')]
        .filter((r) => !r.hidden && !r.closest('[hidden]') && r.offsetParent)
        .map((r) => Math.round(r.getBoundingClientRect().height)).filter((h) => h > 0), screen));
    }
    hs.sort((a, b) => a - b);
    const ceiling = CEILING_COMPACT[screen] + shift;
    const line = `rows ${density.padEnd(9)} ${screen.padEnd(10)} n=${String(hs.length).padStart(3)}  ` +
      `min=${hs[0]}  median=${hs[Math.floor(hs.length / 2)]}  max=${hs[hs.length - 1]}  ` +
      `band ${rowH}–${ceiling}px`;
    if (hs[hs.length - 1] > ceiling) fail(line + `   <-- above the ${ceiling}px ceiling`);
    else if (hs[0] < rowH - 0.5) fail(line + `   <-- below the ${rowH}px --row-h floor`);
    else ok(line);
    await page.selectOption('#pg-' + screen + ' [data-act="state"]', states[0]);
  }
}
await page.evaluate(() => { document.documentElement.setAttribute('data-density', 'compact'); });

/* --- 6. availability accessible names ------------------------------------ */
head('6. Availability glyphs carry a word');
{
  let bad = 0, total = 0;
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    for (const state of await statesOf(page, screen)) {
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(30);
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
  if (bad) fail(`${bad} of ${total} .avail elements have an empty accessible name`);
  else ok(`availability: all ${total} rendered .avail elements carry a word`);
}

/* --- 7. one tab stop per list -------------------------------------------- */
head('7. One tab stop per list (roving tabindex, or a declared opt-out)');
{
  let checked = 0, bad = 0;
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    const states = await statesOf(page, screen);
    for (const state of states) {
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(30);
      const r = await page.evaluate((s) => {
        const out = [];
        document.querySelectorAll('#pg-' + s + ' table[role="table"]').forEach((t) => {
          if (t.closest('[hidden]') || t.hidden) return;
          const rows = [...t.querySelectorAll('tbody tr')].filter((x) => !x.hidden && x.offsetParent);
          if (!rows.length) return;
          out.push({
            label: t.getAttribute('aria-label'),
            optout: t.hasAttribute('data-roving-optout'),
            roving: t.hasAttribute('data-roving'),
            inner: t.querySelectorAll('tbody a[href], tbody button, tbody input, tbody select').length,
            zero: rows.filter((x) => x.tabIndex === 0).length,
          });
        });
        return out;
      }, screen);
      for (const l of r) {
        checked++;
        if (l.optout) continue;
        if (l.inner > 0 && !l.roving) { bad++; fail(`roving: ${screen}/${state} "${l.label}" has ${l.inner} focusable descendants, no data-roving and no declared opt-out`); }
        else if (l.roving && l.zero !== 1) { bad++; fail(`roving: ${screen}/${state} "${l.label}" has ${l.zero} rows at tabindex 0`); }
      }
    }
    await page.selectOption('#pg-' + screen + ' [data-act="state"]', states[0]);
  }
  if (!bad) ok(`roving: every rendered list is one tab stop or declares why not (${checked} list renderings checked)`);
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
head('1b. §13 copy bans, over rendered chrome text (a <td> is data, not copy)');
/* Structural exclusion #3. The copy rules govern the product's own voice.
 * `<td>` content is data — sample data here, database rows in the product —
 * and "BOOM! Studios" is a publisher, not a string UsArr wrote. That exemption
 * is only safe because prose is kept out of cells, which is check 5 above; the
 * two rules hold each other up. */
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
  const fixedBy17 = norm(s17.slice(0, s17.indexOf('\n## ', 10) === -1 ? undefined : s17.indexOf('\n## ', 10)));
  if (fixedBy17.length < 5000) fail('§13 copy: ARCHITECTURE §17 could not be located, so the fixed-wording exemption is not being applied');
  /* What §17 fixes is the CONSTRUCTION, not the instance. Its banner is quoted
   * as "Radarr 4K is unreachable — showing cached data from 14:02"; the mockup
   * renders the same banner over different sample data. So the exemption is
   * granted on the em dash's own two-words-either-side window, which survives a
   * substituted service name and a substituted timestamp and does not survive a
   * rewritten phrase. */
  const exempt = (t) => {
    const w = norm(t).split(' ');
    for (let i = 0; i < w.length; i++) {
      if (w[i] !== '—') continue;
      const win = w.slice(Math.max(0, i - 2), i + 3).join(' ');
      if (win.split(' ').length >= 3 && fixedBy17.includes(win)) return true;
    }
    return false;
  };

  let strings = 0, exempted = 0; const bad = [];
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    const states = await statesOf(page, screen);
    for (const state of states) {
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(20);
      const r = await page.evaluate((s) => {
        /* The unit is a STRING, not a text node. A sentence interrupted by an
         * inline <a> or <code> is one string that happens to be three nodes,
         * and splitting on nodes turns "…retried automatically — and the row
         * says so" into a two-word fragment carrying an em dash. So the unit
         * is the innermost element that lays out as a block: it is exactly one
         * authored string, and it is what a translator would be handed. */
        const out = [];
        const root = document.querySelector('#pg-' + s);
        const BLOCKISH = /^(block|flex|grid|list-item|table-caption)$/;
        root.querySelectorAll('*').forEach((el) => {
          if (el.closest('[hidden]')) return;
          if (el.closest('td')) return;               /* data, not copy */
          if (el.closest('.statebar')) return;        /* the mockup's own scaffolding */
          if (!el.offsetParent && el.tagName !== 'BODY') return;
          if (!BLOCKISH.test(getComputedStyle(el).display)) return;
          if ([...el.children].some((c) => BLOCKISH.test(getComputedStyle(c).display))) return;
          const t = el.textContent.replace(/\s+/g, ' ').trim();
          if (t) out.push(t);
        });
        return out;
      }, screen);
      for (const t of r) {
        strings++;
        const low = t.toLowerCase();
        for (const w of BANNED_WORDS) {
          if (new RegExp('\\b' + w.replace('-', '[- ]') + '\\b').test(low)) bad.push(`${screen}/${state}: banned word "${w}" in "${t.slice(0, 70)}"`);
        }
        if (t.includes('!')) bad.push(`${screen}/${state}: "!" in "${t.slice(0, 70)}"`);
        if (t.includes('—') && t.split(/\s+/).length < 15) {
          if (exempt(t)) exempted++;
          else bad.push(`${screen}/${state}: em dash in a string under 15 words — "${t.slice(0, 70)}"`);
        }
      }
    }
    await page.selectOption('#pg-' + screen + ' [data-act="state"]', states[0]);
  }
  const uniq = [...new Set(bad)];
  if (uniq.length) { fail(`§13 copy: ${uniq.length} violation(s) in rendered chrome text`); uniq.slice(0, 10).forEach((b) => note(b)); }
  else ok(`§13 copy: ${strings} rendered chrome strings clean of banned words, "!" and short-string em dashes ` +
    `(${exempted} short em-dash string(s) exempt because ARCHITECTURE §17 fixes their wording verbatim)`);
}

await browser.close();
console.log(failures ? `\n${failures} FAILURES` : '\nall design checks pass');
process.exit(failures ? 1 : 0);
