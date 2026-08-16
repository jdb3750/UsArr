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
		// scripts/harness/ is the dev-only measurement page driven by
		// scripts/list-bench.mjs. It is a Vite root of its own, outside the
		// SvelteKit project, so `projectService` has no tsconfig that covers it
		// and typed linting errors on the file rather than on its contents.
		// The driver itself (scripts/list-bench.mjs) IS linted.
		ignores: ['build/', '.svelte-kit/', 'node_modules/', 'scripts/harness/']
	}
);
