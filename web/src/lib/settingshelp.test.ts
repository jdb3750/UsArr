/*
 * SETTINGS HELP TEXT DOES NOT PROMISE A KEY THE SPA NEVER HANDLES.
 *
 * The defect this guards against has now shipped twice in the same paragraph.
 * First the "Single-key shortcuts" toggle described a set it did not govern
 * (`shellkeys.test.ts` guards that half). Then the same help text closed with
 * "and `?` always opens the shortcut list" while `web/src` contained no `?`
 * handler and no sheet at all — a control describing, in the present tense,
 * behaviour that does not exist. The sheet is specified
 * (`docs/design/DESIGN-DIRECTION.md:2863-2864`, with the "`?` stays
 * unconditionally" rule at :2878) and `ARCHITECTURE.md` §16 gives it no
 * milestone, so the sentence has to come back WITH the feature.
 *
 * THE RULE, stated so it survives the sheet actually landing: the shortcuts
 * help paragraph may name `?` only when the SPA handles `?`. It is not "never
 * mention `?`" — that would fail the day the sheet ships, which is the day the
 * sentence becomes true.
 *
 * ⚠️ WHAT THE DETECTOR RECOGNISES, AND WHICH WAY IT ERRS. It reads exactly the
 * two shapes this repo uses for a key: a comparison against `.key`
 * (`event.key === '/'`, `shellkeys.ts:62`) and a `case '/':`. A sheet wired
 * some other way — a key map, a library binding — is NOT recognised, and the
 * guard then FAILS with the promise present. That is the safe direction: a
 * detector that goes stale here blocks the sentence rather than waving it
 * through, and whoever ships the sheet in a third shape teaches this file that
 * shape in the same commit. The positive controls below prove the detector and
 * the corpus walk are both alive before any absence is claimed.
 */

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import SETTINGS_SOURCE from '../routes/settings/+page.svelte?raw';

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO = resolve(HERE, '../../..');
const SRC = join(REPO, 'web/src');
const SETTINGS = 'routes/settings/+page.svelte';

/** Comments blanked, line count preserved — `designrules.test.ts`'s `strip`,
 * for the same reason: this rule must never fire on the comment that RECORDS
 * the deleted promise, and that comment quotes it verbatim. */
function stripComments(text: string): string {
	const blank = (m: string) => m.replace(/[^\n]/g, ' ');
	return text
		.replace(/<!--[\s\S]*?-->/g, blank)
		.replace(/\/\*[\s\S]*?\*\//g, blank)
		.replace(/(^|[^:'"\\])\/\/[^\n]*/g, (m, p: string) => p + blank(m.slice(p.length)));
}

/** Every shipped source under `web/src`: a `*.test.ts` is in no bundle, and
 * this file's own prose names `'?'` a dozen times. */
function walk(dir: string, out: string[] = []): string[] {
	for (const entry of readdirSync(dir).sort()) {
		const p = join(dir, entry);
		if (statSync(p).isDirectory()) walk(p, out);
		else if ((p.endsWith('.ts') || p.endsWith('.svelte')) && !p.endsWith('.test.ts')) out.push(p);
	}
	return out;
}

interface Source {
	readonly file: string;
	readonly src: string;
}

const FILES: readonly Source[] = walk(SRC).map((p) => ({
	file: relative(REPO, p),
	src: stripComments(readFileSync(p, 'utf8'))
}));

/** `event.key === 'X'` / `!==` / `case 'X':` — the two shapes, and no others. */
function handlerFor(key: string, sources: readonly Source[]): string[] {
	const lit = key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	const re = new RegExp(`(?:key\\s*[=!]==?\\s*|case\\s+)(['"])${lit}\\1`);
	return sources.filter((s) => re.test(s.src)).map((s) => s.file);
}

describe('the detector and its corpus are alive', () => {
	it('walks a corpus that still contains the shell', () => {
		expect(FILES.length, 'the web/src walk stopped finding sources').toBeGreaterThan(10);
		expect(FILES.map((f) => f.file)).toContain('web/src/lib/shellkeys.ts');
	});

	it('finds the one single-key shortcut the SPA really does handle', () => {
		// `/` is handled, in the exact shape this detector reads. If this line
		// ever goes red the detector is broken, not the app.
		expect(handlerFor('/', FILES), 'no `/` handler found — the detector is stale').toContain(
			'web/src/lib/shellkeys.ts'
		);
	});

	it('does not mistake URL punctuation for a key handler', () => {
		const decoys = [{ file: 'decoy.ts', src: "if (value.includes('?') || url.split('?')[1]) {}" }];
		expect(handlerFor('?', decoys)).toEqual([]);
		expect(
			handlerFor('?', [{ file: 'real.ts', src: "if (event.key === '?') openSheet();" }])
		).toEqual(['real.ts']);
	});
});

describe('the shortcuts help text claims only what is built', () => {
	/** The help paragraph under the toggle, with its offsets asserted rather
	 * than trusted: `indexOf` returning -1 feeds `slice` a negative index and
	 * yields the tail of the file, which is a slice that quietly stops meaning
	 * what it says. */
	const code = stripComments(SETTINGS_SOURCE);
	const from = code.indexOf('id="pref-shortcuts"');
	const to = code.indexOf('</p>', from);

	it('found the paragraph it judges', () => {
		expect(from, `${SETTINGS} no longer renders the shortcuts checkbox`).toBeGreaterThanOrEqual(0);
		expect(to, `${SETTINGS} has no help paragraph after the shortcuts checkbox`).toBeGreaterThan(
			from
		);
		expect(code.slice(from, to), `${SETTINGS} no longer describes the toggle`).toContain(
			'single-key shortcuts as a set'
		);
	});

	it('names `?` only if the SPA handles `?`', () => {
		const help = code.slice(from, to);
		const promises = /\?|shortcut (?:list|sheet)/i.test(help);
		const handlers = handlerFor('?', FILES);
		expect(
			promises && handlers.length === 0,
			`${SETTINGS}'s shortcuts help text describes \`?\` or the shortcut sheet, and no \`?\` ` +
				'handler exists in web/src. Build the sheet (DESIGN-DIRECTION.md:2863-2864) and give ' +
				'it a milestone in ARCHITECTURE §16, or delete the promise. Help text is not where a ' +
				'feature earns its place.'
		).toBe(false);
	});
});
