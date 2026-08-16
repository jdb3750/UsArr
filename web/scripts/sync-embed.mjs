/**
 * Mirror the adapter-static output into the Go package that embeds it.
 *
 * Why this exists: `//go:embed` can only reach files inside the embedding
 * package's own directory, so `internal/web` cannot point at `web/build`.
 * The alternative — building straight into `internal/web/` — would contradict
 * the Makefile (`make clean` removes `web/build`) and docs/DEVELOPMENT.md §2,
 * both of which name `web/build` as the adapter's output. So `web/build` stays
 * canonical and this step copies it one directory over.
 *
 * The destination is wiped first: a stale hashed chunk left behind from an
 * earlier build would be embedded into the binary forever.
 */
import { cp, mkdir, readdir, rm, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const source = join(webDir, 'build');
const destination = resolve(webDir, '..', 'internal', 'web', 'spa');

const entries = await readdir(source).catch(() => {
	throw new Error(`sync-embed: ${source} does not exist — run \`vite build\` first`);
});
if (!entries.includes('index.html')) {
	throw new Error(`sync-embed: ${source} has no index.html — the SPA fallback did not build`);
}

await rm(destination, { recursive: true, force: true });
await mkdir(destination, { recursive: true });
await cp(source, destination, { recursive: true });

// Keeps the directory tracked in git even when no build has run, so
// `//go:embed all:spa` still compiles in a fresh clone. See internal/web/.gitignore.
await writeFile(join(destination, '.gitkeep'), '');

console.log(`sync-embed: ${source} -> ${destination}`);
