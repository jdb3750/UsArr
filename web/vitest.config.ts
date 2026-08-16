import { defineConfig } from 'vitest/config';

// Separate from vite.config.ts on purpose: the SvelteKit plugin has no business
// in a unit-test run, and putting `test` inside a Vite `UserConfig` is a type
// error under Vite 8. The only things tested here are plain modules, imported
// relatively, so no aliases are needed.
export default defineConfig({
	test: {
		include: ['src/**/*.{test,spec}.{js,ts}'],
		environment: 'node'
	}
});
