// Where the camera-effects runtime is served. The folder name changes with
// the MediaPipe version and the model checksums, so files can be cached
// forever (immutable) and an upgrade never mixes old and new files.
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const MODELS = 'https://storage.googleapis.com/mediapipe-models';

export const models = [
  {
    file: 'selfie_segmenter.tflite',
    url: `${MODELS}/image_segmenter/selfie_segmenter/float16/1/selfie_segmenter.tflite`,
    sha256: '191ac9529ae506ee0beefa6b2c945a172dab9d07d1e802a290a4e4038226658b',
  },
  {
    file: 'face_landmarker.task',
    url: `${MODELS}/face_landmarker/face_landmarker/float16/1/face_landmarker.task`,
    sha256: '64184e229b263107bc2b804c6625db1341ff2bb731874b0bcc2fe6544e0bc9ff',
  },
];

const pkg = join(root, 'node_modules', '@mediapipe', 'tasks-vision');
const version = JSON.parse(readFileSync(join(pkg, 'package.json'), 'utf8')).version;
const tag = createHash('sha256')
  .update(models.map((m) => m.sha256).join())
  .digest('hex')
  .slice(0, 8);

export const wasmDir = join(pkg, 'wasm');
export const publicRoot = join(root, 'public', 'vision');
/** e.g. "1.2.3-0a1b2c3d" */
export const visionDir = `${version}-${tag}`;
/** URL path the app loads from. */
export const visionBase = `/vision/${visionDir}`;
