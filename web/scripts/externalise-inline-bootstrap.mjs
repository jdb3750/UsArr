/**
 * Move SvelteKit's inline bootstrap script out of index.html into a real file.
 *
 * WHY THIS EXISTS. The server sends
 *
 *   Content-Security-Policy: default-src 'self'; img-src 'self' data:;
 *     style-src 'self'; script-src 'self'; connect-src 'self';
 *     frame-ancestors 'none'; base-uri 'none'; form-action 'self'
 *
 * (internal/httpapi/middleware.go), with no 'unsafe-inline' and no nonce.
 * adapter-static's fallback document ends with an INLINE <script> that is the
 * only thing that starts the application:
 *
 *   <div style="display: contents">
 *     <script>
 *       { __sveltekit_xxx = { base: "" };
 *         const element = document.currentScript.parentElement;
 *         Promise.all([import(".../start.js"), import(".../app.js")])
 *           .then(([kit, app]) => { kit.start(app, element); }); }
 *     </script>
 *   </div>
 *
 * Measured in Chromium against that exact header, served by the real binary:
 * "Refused to execute inline script because it violates the following Content
 * Security Policy directive: script-src 'self'". The document renders, the
 * stylesheet applies, and NOTHING ELSE HAPPENS — document.body.innerText is
 * empty and no .app element exists. The SPA has never booted under the
 * production CSP; the bug is older than this script and is invisible to
 * `vite preview`, which sends no CSP.
 *
 * SvelteKit has no option to externalise this. `kit.csp` emits hashes into a
 * <meta> policy, which does not help: two policies are enforced by
 * intersection, so a hash in the meta cannot widen the header. So the script is
 * hoisted here, after the build, into a content-hashed file under
 * `_app/immutable/` beside everything else SvelteKit emits.
 *
 * TWO REWRITES ARE REQUIRED and both are load-bearing.
 *   `document.currentScript` is null inside an external script, so the mount
 *   target is looked up by id instead. The id is on the wrapper in app.html.
 *   The tag stays a CLASSIC script, not `type="module"`. SvelteKit's snippet
 *   assigns `__sveltekit_xxx = {...}` with no declaration keyword, which is an
 *   implicit global in sloppy mode and a ReferenceError in a module. `defer`
 *   gives it module-like ordering without module semantics.
 *
 * The step fails the build if an inline script survives it. A silent
 * regression here is a blank page in production and a working one in dev.
 */
import { createHash } from 'node:crypto';
import { readFile, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const indexPath = join(webDir, 'build', 'index.html');
const MOUNT_ID = 'usarr-root';

let html = await readFile(indexPath, 'utf8').catch(() => {
	throw new Error(`externalise-inline-bootstrap: ${indexPath} does not exist — run \`vite build\``);
});

if (!html.includes(`id="${MOUNT_ID}"`)) {
	throw new Error(
		`externalise-inline-bootstrap: no element with id="${MOUNT_ID}" in index.html. ` +
			'src/app.html must wrap %sveltekit.body% in it — the hoisted script looks the ' +
			'mount target up by that id, because document.currentScript is null in an ' +
			'external script.'
	);
}

// A <script> with no src attribute. Matched non-greedily so a document that
// ever carries two of them is handled rather than merged into one.
const INLINE = () => /<script(?![^>]*\ssrc=)([^>]*)>([\s\S]*?)<\/script>/g;

/** [absolute path, source] pairs, written only once every match has succeeded. */
const pending = [];
let hoisted = 0;
html = html.replace(INLINE(), (whole, attrs, body) => {
	if (/\stype=(["'])(?!module|text\/javascript|application\/javascript)/.test(attrs)) {
		// A data block (application/json, importmap, speculationrules). Not code,
		// not covered by script-src's inline rule, and not ours to move.
		return whole;
	}

	const source = body.replace(
		/document\.currentScript\.parentElement/g,
		`document.getElementById(${JSON.stringify(MOUNT_ID)})`
	);
	if (source === body && /currentScript/.test(body)) {
		throw new Error(
			'externalise-inline-bootstrap: the inline script reads document.currentScript in a ' +
				'form this step does not know how to rewrite. It would be null once external. ' +
				'Inspect build/index.html and update the rewrite before shipping.'
		);
	}

	const hash = createHash('sha256').update(source).digest('base64url').slice(0, 8);
	const file = `bootstrap.${hash}.js`;
	pending.push([join(webDir, 'build', '_app', 'immutable', 'entry', file), source]);
	hoisted += 1;
	return `<script defer src="/_app/immutable/entry/${file}"></script>`;
});

if (hoisted === 0) {
	console.log('externalise-inline-bootstrap: no inline script in index.html — nothing to do');
} else {
	for (const [path, source] of pending) await writeFile(path, source);
}

if (INLINE().test(html)) {
	throw new Error(
		"externalise-inline-bootstrap: an inline <script> survived. Under `script-src 'self'` " +
			'the browser refuses it and the application does not start at all.'
	);
}

await writeFile(indexPath, html);
console.log(`externalise-inline-bootstrap: hoisted ${hoisted} inline script(s) out of index.html`);
