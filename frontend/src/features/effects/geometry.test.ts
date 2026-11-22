import { faceGeometry, type Pt } from './geometry';

/** A level face: eyes at y=0.4, cheeks 0.4 apart, forehead at y=0.2. */
function face(tilt = 0): Pt[] {
  const lm: Pt[] = Array.from({ length: 478 }, () => ({ x: 0.5, y: 0.5 }));
  const set = (i: number, x: number, y: number) => (lm[i] = { x, y: y + tilt * (x - 0.5) });
  set(33, 0.38, 0.4);
  set(133, 0.44, 0.4);
  set(263, 0.62, 0.4);
  set(362, 0.56, 0.4);
  set(234, 0.3, 0.45);
  set(454, 0.7, 0.45);
  set(10, 0.5, 0.2);
  set(1, 0.5, 0.5);
  return lm;
}

describe('faceGeometry', () => {
  it('measures a level face in pixels', () => {
    const f = faceGeometry(face(), 1000, 1000)!;
    expect(f.angle).toBeCloseTo(0);
    expect(f.width).toBeCloseTo(400);
    expect(f.eyeMid).toEqual({ x: 500, y: 400 });
    expect(f.eyeDist).toBeCloseTo(180);
    expect(f.top).toEqual({ x: 500, y: 200 });
  });

  it('follows head tilt', () => {
    const f = faceGeometry(face(1), 1000, 1000)!;
    expect(f.angle).toBeCloseTo(Math.PI / 4);
  });

  it('rejects incomplete or tiny faces', () => {
    expect(faceGeometry(face().slice(0, 100), 1000, 1000)).toBeNull();
    expect(faceGeometry(face(), 10, 10)).toBeNull();
  });
});
