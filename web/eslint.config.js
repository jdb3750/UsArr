import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import ts from 'typescript-eslint';
import svelteConfig from './svelte.config.js';

export default ts.config(
	js.configs.recommended,
	...ts.configs.recommended,
	...svelte.configs.recommended,
	{
		languageOptions: {
			globals: { ...globals.browser, ...globals.node }
		}
	},
	{
		files: ['**/*.svelte', '**/*.svelte.ts', '**/*.svelte.js'],
		languageOptions: {
			parserOptions: {
				projectService: true,
				extraFileExtensions: ['.svelte'],
				parser: ts.parser,
				svelteConfig
			}
		}
	},
	{
		// scripts/harness/ and scripts/freeze-harness/ are the dev-only pages
		// driven by scripts/list-bench.mjs and scripts/freeze-check.mjs. Each is
		// a Vite root of its own, outside the SvelteKit project, so
		// `projectService` has no tsconfig that covers it and typed linting
		// errors on the file rather than on its contents. Both DRIVERS are
		// linted, and they are where the logic lives.
		ignores: [
			'build/',
			'.svelte-kit/',
			'node_modules/',
			'scripts/harness/',
			'scripts/freeze-harness/'
		]
	}
);
