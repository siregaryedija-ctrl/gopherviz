// Bundel ekstensi dengan esbuild. Webview (media/) tidak dibundel: file JS-nya
// dimuat apa adanya oleh panel.
//   node scripts/build.mjs          build produksi ke dist/
//   node scripts/build.mjs --watch  build ulang otomatis saat src berubah
//   node scripts/build.mjs --test   bundel test ke out/test/
import * as esbuild from 'esbuild';

const watch = process.argv.includes('--watch');
const test = process.argv.includes('--test');

const common = {
  bundle: true,
  platform: 'node',
  format: 'cjs',
  target: 'node20',
  sourcemap: true,
  external: ['vscode'],
  logLevel: 'info',
};

if (test) {
  await esbuild.build({ ...common, entryPoints: ['test/server.test.ts'], outdir: 'out/test' });
} else {
  const ctx = await esbuild.context({
    ...common,
    entryPoints: ['src/extension.ts'],
    outfile: 'dist/extension.js',
    minify: !watch,
  });
  if (watch) {
    await ctx.watch();
  } else {
    await ctx.rebuild();
    await ctx.dispose();
  }
}
