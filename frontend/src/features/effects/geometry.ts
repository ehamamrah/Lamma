export interface Pt {
  x: number;
  y: number;
}

/** Where a face is in the frame, in pixels. */
export interface Face {
  eyeMid: Pt;
  eyeDist: number;
  nose: Pt; // tip
  underNose: Pt; // between the nose and the upper lip
  cheeks: [Pt, Pt];
  top: Pt; // top of the forehead
  /** Head roll in radians (0 = level). */
  angle: number;
  /** Cheek-to-cheek width. Masks are sized from this. */
  width: number;
}

// Indices in MediaPipe's 478-point face mesh.
const L = {
  eyeA: [33, 133],
  eyeB: [263, 362],
  noseTip: 1,
  subnasale: 2,
  upperLip: 13,
  cheekA: 50,
  cheekB: 280,
  forehead: 10,
  sideA: 234,
  sideB: 454,
} as const;

const dist = (a: Pt, b: Pt) => Math.hypot(b.x - a.x, b.y - a.y);
const mid = (a: Pt, b: Pt): Pt => ({ x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 });

/** Converts normalised landmarks to the points the masks are drawn from. */
export function faceGeometry(lm: readonly Pt[], w: number, h: number): Face | null {
  if (lm.length < 468) return null;
  const p = (i: number): Pt => {
    const q = lm[i] ?? { x: 0, y: 0 };
    return { x: q.x * w, y: q.y * h };
  };
  const eyeA = mid(p(L.eyeA[0]), p(L.eyeA[1]));
  const eyeB = mid(p(L.eyeB[0]), p(L.eyeB[1]));
  const width = dist(p(L.sideA), p(L.sideB));
  if (width < 8) return null;
  return {
    eyeMid: mid(eyeA, eyeB),
    eyeDist: dist(eyeA, eyeB),
    nose: p(L.noseTip),
    underNose: mid(p(L.subnasale), p(L.upperLip)),
    cheeks: [p(L.cheekA), p(L.cheekB)],
    top: p(L.forehead),
    angle: Math.atan2(eyeB.y - eyeA.y, eyeB.x - eyeA.x),
    width,
  };
}
