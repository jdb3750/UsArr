/* Self-tests for the UsArr screen mockup.
 *
 *   PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers node selftest.mjs
 *
 * Everything here is a check that a previous review round found by hand and
 * that a machine can find in three seconds. Each one is written as an
 * assertion over the rendered DOM rather than over the source, because every
 * defect it catches was invisible in the source:
 *
 *   1. OVERFLOW, at two scopes. No element's right edge may exceed innerWidth,
 *      and no .tablewrap descendant's right edge may exceed its own wrapper,
 *      on every screen x state x width. The two fix-it buttons on Services --
 *      the only controls on that screen that repair a broken service -- were
 *      sheared off flush with the page edge at 1280, 1440, 1680 and 1920, with
 *      document.scrollWidth equal to the viewport at every one of them, so
 *      there was not even a scrollbar to reach them with.
 *
 *      The second scope is the one that matters now and it is not implied by
 *      the first. .tablewrap is `overflow-x: clip`, which amputates rather
 *      than degrades, and a wrapper can be narrower than the viewport -- the
 *      Home grid is two columns -- so a cell can be sheared by its wrapper
 *      while sitting comfortably inside innerWidth. This assertion is what the
 *      briefly-tried `overflow-x: auto` was standing in for, and it is the
 *      better guard: it fails the build rather than quietly growing a
 *      scrollbar and, as a side effect, killing the sticky header by turning
 *      every wrapper into a scroll container.
 *   2. AVAILABILITY NAMES. No .avail element may have an empty accessible
 *      name. 62 of them shipped as a bare icon, so a screen reader heard
 *      identical silence for "have" and "missing" -- the product's central
 *      question, unanswerable -- while the Settings screen printed a sentence
 *      claiming every status is an icon and a word as well as a colour.
 *   3. ROVING. Every list with role="table" and focusable descendants must
 *      have exactly one row at tabindex="0". The three Requests tables had
 *      zero, on the one v0.1 screen with a stateful outbound action.
 *   4. ROW HEIGHT. Services and Libraries rows must sit in the same band as
 *      Home and Search. They ran to 160px against a 28px compact standard,
 *      because prose was living in table cells.
 *   5. WEBFONT. The document must actually be rendering IBM Plex. The stack
 *      named it for months while no @font-face existed, so every review to
 *      date was conducted in DejaVu Sans.
 */
import { chromium } from '/opt/node22/lib/node_modules/playwright/index.mjs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const HERE = dirname(fileURLToPath(import.meta.url));
const URL = 'file://' + join(HERE, 'prototype.html');
const SCREENS = ['home', 'services', 'libraries', 'search', 'requests'];
const WIDTHS = [390, 1280, 1440, 1680, 1920];

let failures = 0;
const fail = (msg) => { failures++; console.log('FAIL  ' + msg); };
const ok = (msg) => console.log('ok    ' + msg);

const browser = await chromium.launch();

/* ---- 1. overflow, every screen x state x width ------------------------- */
for (const width of WIDTHS) {
  const ctx = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await ctx.newPage();
  await page.goto(URL);
  let worstOver = 0, worstWhere = '';
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(60);
    const states = await page.$$eval('#pg-' + screen + ' [data-act="state"] option',
      (os) => os.map((o) => o.value));
    for (const state of states) {
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(40);
      const r = await page.evaluate((s) => {
        const main = document.querySelector('#pg-' + s);
        let over = 0, worst = 0, ex = [];
        main.querySelectorAll('*').forEach((el) => {
          const b = el.getBoundingClientRect();
          if (!b.width || b.right <= window.innerWidth + 0.5) return;
          over++;
          if (b.right > worst) worst = b.right;
          if (ex.length < 4) ex.push((el.className || el.tagName) + ' r=' + Math.round(b.right) +
            ' "' + (el.textContent || '').trim().slice(0, 30) + '"');
        });
        /* Clipped by the wrapper rather than by the page. */
        const clip = [];
        main.querySelectorAll('.tablewrap').forEach((w) => {
          if (w.closest('[hidden]') || !w.offsetParent) return;
          const edge = w.getBoundingClientRect().right;
          w.querySelectorAll('*').forEach((el) => {
            if (el.closest('[hidden]')) return;
            const b = el.getBoundingClientRect();
            if (!b.width || b.right <= edge + 0.5) return;
            if (clip.length < 4) clip.push((el.className || el.tagName) +
              ' +' + Math.round(b.right - edge) + 'px past the wrapper "' +
              (el.textContent || '').trim().slice(0, 30) + '"');
          });
        });
        return { over, worst: Math.round(worst), ex, clip,
                 doc: document.documentElement.scrollWidth };
      }, screen);
      if (r.clip.length) {
        fail(`${width}px ${screen}/${state}: sheared by overflow-x:clip on .tablewrap\n        ` +
          r.clip.join('\n        '));
      }
      if (r.over) {
        worstOver = Math.max(worstOver, r.worst - width);
        worstWhere = `${width}px ${screen}/${state}: ${r.over} over, worst right=${r.worst}\n        ${r.ex.join('\n        ')}`;
      }
      if (r.doc > width) {
        fail(`${width}px ${screen}/${state}: document scrolls sideways (${r.doc})`);
      }
    }
  }
  if (worstOver > 0) fail('overflow ' + worstWhere);
  else ok(`overflow: nothing past ${width}px, and nothing past its .tablewrap, on any screen in any state`);
  await ctx.close();
}

/* ---- 2, 3, 4, 5 at the reference desktop width ------------------------- */
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
await page.goto(URL);
await page.waitForTimeout(200);

/* availability names, across every screen and state */
{
  let bad = 0, total = 0;
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    const states = await page.$$eval('#pg-' + screen + ' [data-act="state"] option', (os) => os.map((o) => o.value));
    for (const state of states) {
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

/* Roving: every list of data rows is one tab stop. A grid whose every cell is
 * a form control is a form and keeps its natural tab order -- but that has to
 * be a declared decision, because from outside it is indistinguishable from
 * the omission that left the release tables with 28 tab stops and no rows. */
{
  let checked = 0, bad = 0;
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    const states = await page.$$eval('#pg-' + screen + ' [data-act="state"] option', (os) => os.map((o) => o.value));
    for (const state of states) {
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(30);
      const r = await page.evaluate((s) => {
        const out = [];
        document.querySelectorAll('#pg-' + s + ' table[role="table"]').forEach((t) => {
          if (t.closest('[hidden]') || t.hidden) return;
          const rows = [...t.querySelectorAll('tbody tr')].filter((x) => !x.hidden && x.offsetParent);
          if (!rows.length) return;
          const inner = t.querySelectorAll('tbody a[href], tbody button, tbody input, tbody select').length;
          out.push({
            label: t.getAttribute('aria-label'), optout: t.hasAttribute('data-roving-optout'),
            roving: t.hasAttribute('data-roving'), inner,
            zero: rows.filter((x) => x.tabIndex === 0).length
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

/* Row heights, per screen, over every state the screen can be in. Services and
 * Libraries ran 80-165px against a 28px compact standard because prose was
 * living in table cells; the ceiling is 48px, which is two lines of body text
 * plus the compact row padding. The annex on Services is excluded by name: it
 * is explicitly labelled as documentation for services v0.1 does not have. */
{
  const heights = {};
  for (const screen of SCREENS) {
    await page.evaluate((s) => { window.location.hash = '#' + s; }, screen);
    await page.waitForTimeout(50);
    const states = await page.$$eval('#pg-' + screen + ' [data-act="state"] option', (os) => os.map((o) => o.value));
    const hs = [];
    for (const state of states) {
      if (state === 'annex') continue;
      await page.selectOption('#pg-' + screen + ' [data-act="state"]', state);
      await page.waitForTimeout(30);
      hs.push(...await page.evaluate((s) => [...document.querySelectorAll('#pg-' + s + ' tbody tr')]
        .filter((r) => !r.hidden && !r.closest('[hidden]') && r.offsetParent)
        .map((r) => Math.round(r.getBoundingClientRect().height)).filter((h) => h > 0), screen));
    }
    hs.sort((a, b) => a - b);
    heights[screen] = { n: hs.length, min: hs[0], median: hs[Math.floor(hs.length / 2)], max: hs[hs.length - 1] };
    await page.selectOption('#pg-' + screen + ' [data-act="state"]', states[0]);
  }
  /* Services and Libraries are the two screens the row-height finding was
   * filed against and they carry the tight ceiling: 48px of content plus the
   * 1px row rule. Home, Search and Requests are held to a looser one -- their
   * tallest rows are a track listing and a release row carrying a two-line
   * confirmation sentence, which is content doing work rather than prose that
   * escaped into a cell. */
  const CEILING = { services: 49, libraries: 49, home: 60, search: 80, requests: 80 };
  for (const [k, v] of Object.entries(heights)) {
    const line = `rows ${k.padEnd(10)} n=${String(v.n).padStart(3)}  min=${v.min}  median=${v.median}  max=${v.max}`;
    if (v.max > CEILING[k]) fail(line + `   <-- above the ${CEILING[k]}px ceiling`);
    else ok(line);
  }
}

/* the webfont */
{
  const face = await page.evaluate(() => {
    const c = document.createElement('canvas').getContext('2d');
    const w = (f) => { c.font = '40px ' + f; return c.measureText('Handgloves 0123456789').width.toFixed(3); };
    return {
      plexSans: w('"IBM Plex Sans"'),
      plexMono: w('"IBM Plex Mono"'),
      bogus: w('"ZZZNoSuchFaceZZZ"'),
      uiStack: w(getComputedStyle(document.body).fontFamily),
      loaded: [...document.fonts].map((f) => f.family + ' ' + f.weight + ' ' + f.status).join(' | ')
    };
  });
  if (face.plexSans === face.bogus) fail('webfont: IBM Plex Sans is not resolving (identical width to a bogus family)');
  else ok(`webfont: IBM Plex Sans resolves (${face.plexSans}px vs bogus ${face.bogus}px), body renders at ${face.uiStack}px`);
  if (face.plexMono === face.bogus) fail('webfont: IBM Plex Mono is not resolving');
  else ok(`webfont: IBM Plex Mono resolves (${face.plexMono}px)`);
  console.log('      document.fonts: ' + face.loaded);
}

await browser.close();
console.log(failures ? `\n${failures} FAILURES` : '\nall self-tests pass');
process.exit(failures ? 1 : 0);
