import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/**
 * UsArr ships one static binary. The frontend is a pure SPA (ADR-0003): no Node
 * process in production, no SSR, one fallback document served at every depth by
 * `internal/web`.
 *
 * Three settings below are load-bearing and are documented in ADR-0024 §6.
 *
 *   pages/assets = 'build'  The Makefile (`make clean`, `make web-build`) and
 *                           docs/DEVELOPMENT.md §2 both name `web/build`. This
 *                           is the adapter's default; it is stated explicitly
 *                           so a future edit has to be deliberate.
 *
 *   fallback = 'index.html' What makes this an SPA rather than a prerendered
 *                           site. Every unmatched route resolves client-side.
 *
 *   paths.base = ''         Baked in at build time, so UsArr must be proxied at
 *                           a domain or subdomain root, never a subpath.
 *
 *   paths.relative = false  ADR-0024 flagged this as untested inference and
 *                           asked for it to be tested before the config froze.
 *                           IT HAS BEEN TESTED, AND THE INFERENCE IS WRONG.
 *                           SvelteKit already special-cases the fallback:
 *                           `src/runtime/server/page/render.js` (2.70.2) reads
 *
 *                               if (paths.relative) {
 *                                 if (!state.prerendering?.fallback) { ... }
 *
 *                           so the relative-path rewrite is skipped entirely
 *                           for the fallback document. Built both ways here,
 *                           index.html emits root-absolute `/_app/...` either
 *                           way, and `/search` loads. `relative` still governs
 *                           genuinely prerendered pages, of which this SPA has
 *                           none. It is pinned to false anyway — explicit beats
 *                           relying on a special case, and it costs nothing —
 *                           but it is NOT load-bearing. ADR-0024 §6 should be
 *                           corrected. internal/web/web_test.go asserts the
 *                           property that actually matters.
 *
 * precompress is deliberately OFF. adapter-static can emit .br/.gz, but Go's
 * stock http.FileServer will not serve them and internal/web does not carry a
 * precompression-aware handler (statigz or similar) yet. Emitting files nothing
 * serves is worse than not emitting them.
 */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			precompress: false,
			strict: true
		}),
		paths: {
			base: '',
			relative: false
		}
	}
};

export default config;
