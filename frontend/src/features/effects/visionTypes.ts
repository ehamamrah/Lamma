/** Messages between the page and the vision worker. */

export type ModelKey = 'segmenter' | 'face';
export type ModelStatus = 'idle' | 'loading' | 'ready' | 'error';

export interface Landmark {
  x: number;
  y: number;
}

export type VisionRequest =
  | { type: 'init'; base: string }
  | { type: 'frame'; id: number; bitmap: ImageBitmap; segment: boolean; faces: boolean };

export interface PersonMask {
  data: Float32Array; // 0..1 "is a person", row-major
  width: number;
  height: number;
}

export type VisionReply =
  | { type: 'status'; key: ModelKey; status: ModelStatus }
  | { type: 'result'; id: number; mask?: PersonMask; faces?: Landmark[][] };
