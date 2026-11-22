import { create } from 'zustand';
import type { ModelStatus, VisionReply } from './visionTypes';

type Result = Extract<VisionReply, { type: 'result' }>;

/** Load state of the two models, for the picker to show. */
export const useVisionStatus = create<{ segmenter: ModelStatus; face: ModelStatus }>(() => ({
  segmenter: 'idle',
  face: 'idle',
}));

// Served by our own Nginx (see scripts/vision-assets.mjs), never a CDN.
// The folder is versioned, so it can be cached forever.
declare const __VISION_BASE__: string;

let worker: Worker | null = null;
let nextId = 0;
const pending = new Map<number, (r: Result) => void>();

/** One worker for the whole tab, so preview and meeting share the models. */
function getWorker(): Worker {
  if (worker) return worker;
  worker = new Worker(new URL('./vision.worker.ts', import.meta.url), { type: 'module' });
  worker.onmessage = (e: MessageEvent<VisionReply>) => {
    const m = e.data;
    if (m.type === 'status') useVisionStatus.setState({ [m.key]: m.status });
    else {
      pending.get(m.id)?.(m);
      pending.delete(m.id);
    }
  };
  worker.onerror = () => {
    useVisionStatus.setState({ segmenter: 'error', face: 'error' });
    for (const done of pending.values()) done({ type: 'result', id: -1 });
    pending.clear();
  };
  worker.postMessage({ type: 'init', base: __VISION_BASE__ });
  return worker;
}

/**
 * Asks the worker for the person mask and/or face landmarks of one frame.
 * The bitmap is transferred (and closed by the worker). Models load on the
 * first request; until then the result is empty.
 */
export function analyse(bitmap: ImageBitmap, want: { segment: boolean; faces: boolean }) {
  const id = ++nextId;
  return new Promise<Result>((resolve) => {
    pending.set(id, resolve);
    getWorker().postMessage({ type: 'frame', id, bitmap, ...want }, [bitmap]);
  });
}
