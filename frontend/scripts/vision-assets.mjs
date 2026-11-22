// Puts the camera-effects runtime in public/vision/<version>/ so it is
// served from our own origin (no third-party CDN, and the CSP stays 'self'):
//   - the MediaPipe wasm runtime, copied from node_modules
//   - two models, downloaded once and verified against pinned checksums
// The runtime gets a .gz next to it, which Nginx serves as-is (gzip_static)
// instead of compressing on every request.
import { createHash } from 'node:crypto';
import { copyFile, mkdir, readFile, readdir, rename, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { promisify } from 'node:util';
import { gzip } from 'node:zlib';
import { models, publicRoot, visionDir, wasmDir } from './vision-paths.mjs';

const out = join(publicRoot, visionDir);
const sha256 = (buf) => createHash('sha256').update(buf).digest('hex');

async function ensureModel({ file, url, sha256: want }) {
  const path = join(out, file);
  const existing = await readFile(path).catch(() => null);
  if (existing && sha256(existing) === want) return;
  const res = await fetch(url, { signal: AbortSignal.timeout(120_000) });
  if (!res.ok) throw new Error(`${file}: download failed (${res.status})`);
  const body = Buffer.from(await res.arrayBuffer());
  const got = sha256(body);
  if (got !== want) throw new Error(`${file}: checksum mismatch (got ${got})`);
  await writeFile(`${path}.tmp`, body);
  await rename(`${path}.tmp`, path);
  console.log(`vision: downloaded ${file}`);
}

async function precompress(file) {
  const path = join(out, file);
  const gz = await promisify(gzip)(await readFile(path), { level: 9 });
  await writeFile(`${path}.gz`, gz);
}

await mkdir(out, { recursive: true });
// Drop folders from older versions.
for (const entry of await readdir(publicRoot)) {
  if (entry !== visionDir) await rm(join(publicRoot, entry), { recursive: true, force: true });
}
const runtime = ['vision_wasm_internal.js', 'vision_wasm_internal.wasm'];
for (const f of runtime) await copyFile(join(wasmDir, f), join(out, f));
await Promise.all(models.map(ensureModel));
// The models barely compress, so only the runtime gets a .gz.
await Promise.all(runtime.map(precompress));
