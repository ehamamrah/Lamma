import type { FaceLandmarker, ImageSegmenter } from '@mediapipe/tasks-vision';
import { create } from 'zustand';

export type ModelStatus = 'idle' | 'loading' | 'ready' | 'error';

/** Load state of the two models, for the picker to show. */
export const useVisionStatus = create<{ segmenter: ModelStatus; face: ModelStatus }>(() => ({
  segmenter: 'idle',
  face: 'idle',
}));

// Served by our own Nginx (see scripts/vision-assets.mjs), never a CDN.
// The folder is versioned, so it can be cached forever.
declare const __VISION_BASE__: string;
const BASE = __VISION_BASE__;
const files = {
  wasmLoaderPath: `${BASE}/vision_wasm_internal.js`,
  wasmBinaryPath: `${BASE}/vision_wasm_internal.wasm`,
};

/**
 * A model that loads the first time it is asked for. Until it is ready,
 * `get()` returns null and callers fall back (the library is code-split,
 * so people who never use effects never download it).
 */
function lazyModel<T>(key: 'segmenter' | 'face', load: () => Promise<T>) {
  let instance: T | null = null;
  return () => {
    if (useVisionStatus.getState()[key] === 'idle') {
      useVisionStatus.setState({ [key]: 'loading' });
      load().then(
        (m) => {
          instance = m;
          useVisionStatus.setState({ [key]: 'ready' });
        },
        (err: unknown) => {
          console.warn(`camera effects: ${key} failed to load`, err);
          useVisionStatus.setState({ [key]: 'error' });
        },
      );
    }
    return instance;
  };
}

// MediaPipe's video mode needs strictly increasing timestamps per model.
// The models are shared, so the clock is too.
let lastTs = 0;
export const nextTimestamp = () => (lastTs = Math.max(performance.now(), lastTs + 1));

const tasks = () => import('@mediapipe/tasks-vision');

export const getSegmenter = lazyModel<ImageSegmenter>('segmenter', async () =>
  (await tasks()).ImageSegmenter.createFromOptions(files, {
    baseOptions: { modelAssetPath: `${BASE}/selfie_segmenter.tflite`, delegate: 'CPU' },
    runningMode: 'VIDEO',
    outputConfidenceMasks: true,
    outputCategoryMask: false,
  }),
);

export const getFaceLandmarker = lazyModel<FaceLandmarker>('face', async () =>
  (await tasks()).FaceLandmarker.createFromOptions(files, {
    baseOptions: { modelAssetPath: `${BASE}/face_landmarker.task`, delegate: 'CPU' },
    runningMode: 'VIDEO',
    numFaces: 3,
  }),
);
