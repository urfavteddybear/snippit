// Builds frontend assets into internal/web/static, which the Go binary embeds.
import * as esbuild from 'esbuild';
import { spawn } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';

const out = '../internal/web/static';
const watch = process.argv.includes('--watch');

rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const builds = [
  {
    entryPoints: { app: 'src/app.js' },
    bundle: true,
    format: 'esm',
    splitting: true,
    minify: true,
    target: 'es2020',
    chunkNames: 'chunks/[name]-[hash]',
    legalComments: 'none',
    outdir: out,
  },
  {
    entryPoints: { fonts: 'src/fonts.css' },
    bundle: true,
    minify: true,
    loader: { '.woff2': 'file' },
    assetNames: 'fonts/[name]-[hash]',
    outdir: out,
  },
];

const tailwindArgs = ['-i', 'src/app.css', '-o', `${out}/app.css`, '--minify'];

if (watch) {
  for (const b of builds) await (await esbuild.context(b)).watch();
  spawn('node_modules/.bin/tailwindcss', [...tailwindArgs, '--watch'], { stdio: 'inherit' });
} else {
  await Promise.all(builds.map((b) => esbuild.build(b)));
  await new Promise((resolve, reject) => {
    const p = spawn('node_modules/.bin/tailwindcss', tailwindArgs, { stdio: 'inherit' });
    p.on('exit', (code) => (code === 0 ? resolve() : reject(new Error(`tailwindcss exited with ${code}`))));
  });
}
