import { build } from 'esbuild';
import { mkdir, copyFile, readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const dist = dirname(require.resolve('jassub'));
const output = resolve(import.meta.dirname, '../public/subtitles');
await mkdir(output, { recursive: true });
await build({ entryPoints: [resolve(dist, 'worker/worker.js')], outfile: resolve(output, 'jassub-worker.js'), bundle: true, format: 'esm', platform: 'browser', target: 'es2022', plugins: [{
  name: 'subtitle-canvas-compositor',
  setup(build) {
    build.onLoad({ filter: /worker\/worker\.js$/ }, async ({ path }) => {
      const source = await readFile(path, 'utf8');
      const probe = 'const testCanvas = new OffscreenCanvas(1, 1);';
      // OffscreenCanvas WebGL and hardware video planes can composite incorrectly
      // in embedded browsers when sibling menus disappear. Keep libass in its
      // worker, but use the transparent Canvas2D renderer on every browser.
      if (!source.includes(probe)) throw new Error('Review JASSUB Canvas2D compatibility patch after upgrade');
      return { contents: source.replace(probe, 'const testCanvas = { getContext: () => null };'), loader: 'js' };
    });
  },
}] });
for (const file of ['jassub-worker.wasm', 'jassub-worker-modern.wasm']) {
  await copyFile(resolve(dist, 'wasm', file), resolve(output, file));
}
await copyFile(resolve(dist, 'default.woff2'), resolve(output, 'default.woff2'));
await copyFile(resolve(dist, '../LICENSE'), resolve(output, 'LICENSE'));
