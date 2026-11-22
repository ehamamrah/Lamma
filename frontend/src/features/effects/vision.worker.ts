// Runs MediaPipe off the main thread, so effects can never freeze the
// meeting UI: the page sends a small frame, this worker answers with the
// person mask and face landmarks.
import { FaceLandmarker, ImageSegmenter } from '@mediapipe/tasks-vision';
import type { ModelKey, PersonMask, VisionReply, VisionRequest } from './visionTypes';

// The app is typed against the DOM; describe just what we use of the worker scope.
const scope = self as unknown as {
  postMessage(msg: VisionReply, transfer: Transferable[]): void;
  onmessage: ((e: MessageEvent<VisionRequest>) => void) | null;
};

const RETRY_MS = [2_000, 8_000];
let base = '';
let lastTs = 0;
// Video mode needs strictly increasing timestamps per model.
const nextTimestamp = () => (lastTs = Math.max(performance.now(), lastTs + 1));
const post = (msg: VisionReply, transfer: Transferable[] = []) => scope.postMessage(msg, transfer);

// The ES-module build of the runtime: module workers cannot importScripts.
const files = () => ({
  wasmLoaderPath: `${base}/vision_wasm_module_internal.js`,
  wasmBinaryPath: `${base}/vision_wasm_module_internal.wasm`,
});

/**
 * Loads a model the first time it is asked for, retrying twice, and reports
 * progress to the page. Returns null until it is ready.
 */
function lazyModel<T>(key: ModelKey, load: () => Promise<T>) {
  let instance: T | null = null;
  let started = false;
  let failures = 0;
  const attempt = () => {
    post({ type: 'status', key, status: 'loading' });
    load().then(
      (m) => {
        instance = m;
        post({ type: 'status', key, status: 'ready' });
      },
      (err: unknown) => {
        console.warn(`camera effects: ${key} failed to load`, err);
        const wait = RETRY_MS[failures++];
        if (wait === undefined) post({ type: 'status', key, status: 'error' });
        else setTimeout(attempt, wait);
      },
    );
  };
  return () => {
    if (!started) {
      started = true;
      attempt();
    }
    return instance;
  };
}

const segmenter = lazyModel('segmenter', () =>
  ImageSegmenter.createFromOptions(files(), {
    baseOptions: { modelAssetPath: `${base}/selfie_segmenter.tflite`, delegate: 'CPU' },
    runningMode: 'VIDEO',
    outputConfidenceMasks: true,
    outputCategoryMask: false,
  }),
);

const faceLandmarker = lazyModel('face', () =>
  FaceLandmarker.createFromOptions(files(), {
    baseOptions: { modelAssetPath: `${base}/face_landmarker.task`, delegate: 'CPU' },
    runningMode: 'VIDEO',
    numFaces: 3,
  }),
);

function segment(bitmap: ImageBitmap): PersonMask | undefined {
  let mask: PersonMask | undefined;
  segmenter()?.segmentForVideo(bitmap, nextTimestamp(), (r) => {
    const m = r.confidenceMasks?.[0];
    // Copy: the result is only valid inside this callback.
    if (m) mask = { data: m.getAsFloat32Array().slice(), width: m.width, height: m.height };
  });
  return mask;
}

function analyse(msg: Extract<VisionRequest, { type: 'frame' }>) {
  try {
    const mask = msg.segment ? segment(msg.bitmap) : undefined;
    const faces = msg.faces
      ? faceLandmarker()
          ?.detectForVideo(msg.bitmap, nextTimestamp())
          .faceLandmarks.map((f) => f.map(({ x, y }) => ({ x, y })))
      : undefined;
    post({ type: 'result', id: msg.id, mask, faces }, mask ? [mask.data.buffer] : []);
  } catch (err) {
    console.warn('camera effects: analysis failed', err);
    post({ type: 'result', id: msg.id });
  } finally {
    msg.bitmap.close();
  }
}

scope.onmessage = (e) => {
  if (e.data.type === 'init') base = e.data.base;
  else analyse(e.data);
};
