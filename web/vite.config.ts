import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// `make web-dev` runs this on :5173 while `make dev` runs the Go backend on
// :8484 (docs/DEVELOPMENT.md §3). The proxy makes the two look like one origin
// so the SPA's fetch and EventSource URLs are identical in dev and in the
// embedded build — no base-URL switch anywhere in src/.
export default defineConfig({
	plugins: [sveltekit()],
	server: {
		port: 5173,
		strictPort: true,
		proxy: {
			'/api': {
				target: 'http://localhost:8484',
				changeOrigin: true,
				// /api/events is Server-Sent Events. Buffering it would defeat the
				// whole point of the stream, so responses are passed straight
				// through and the socket is allowed to stay open indefinitely.
				timeout: 0,
				proxyTimeout: 0
			}
		}
	}
});
