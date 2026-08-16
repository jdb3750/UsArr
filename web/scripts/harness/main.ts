/**
 * Harness entry point. Mounts one List and hangs the driver handle off window.
 *
 * `prefs` is imported for the same side effect the real shell relies on —
 * stamping data-theme / data-density on <html> before anything renders — so the
 * harness measures the same starting state the application does.
 */
import '../../src/app.css';
import { mount, flushSync } from 'svelte';
import Harness from './Harness.svelte';
import { harness } from './state.svelte';
import { prefs } from '../../src/lib/prefs.svelte';

declare global {
	interface Window {
		__harness: {
			harness: typeof harness;
			prefs: typeof prefs;
			/** Apply pending state changes synchronously, so a measurement that
			 * follows is measuring the DOM the driver asked for. */
			flush: () => void;
		};
	}
}

mount(Harness, { target: document.getElementById('harness')! });

window.__harness = {
	harness,
	prefs,
	flush: () => flushSync()
};
