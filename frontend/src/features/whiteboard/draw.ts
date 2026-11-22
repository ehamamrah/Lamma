import type { Stroke } from '@/features/collab/protocol';

export const BOARD_RATIO = 16 / 9;

/** Redraws every stroke; points are normalised, widths in board thousandths. */
export function drawStrokes(
  ctx: CanvasRenderingContext2D,
  strokes: Stroke[],
  w: number,
  h: number,
) {
  ctx.clearRect(0, 0, w, h);
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
  for (const s of strokes) drawStroke(ctx, s, w, h);
}

function drawStroke(ctx: CanvasRenderingContext2D, s: Stroke, w: number, h: number) {
  const [x0 = 0, y0 = 0] = s.points;
  ctx.strokeStyle = s.color;
  ctx.fillStyle = s.color;
  ctx.lineWidth = (s.width / 1000) * w;
  if (s.points.length === 2) {
    ctx.beginPath();
    ctx.arc(x0 * w, y0 * h, ctx.lineWidth / 2, 0, Math.PI * 2);
    ctx.fill();
    return;
  }
  ctx.beginPath();
  ctx.moveTo(x0 * w, y0 * h);
  for (let i = 2; i < s.points.length; i += 2) {
    ctx.lineTo((s.points[i] ?? 0) * w, (s.points[i + 1] ?? 0) * h);
  }
  ctx.stroke();
}

/** Largest 16:9 box that fits inside the container. */
export function fitBoard(cw: number, ch: number): { w: number; h: number } {
  const w = Math.min(cw, ch * BOARD_RATIO);
  return { w: Math.floor(w), h: Math.floor(w / BOARD_RATIO) };
}

/** Pointer position normalised to 0..1, rounded to keep messages small. */
export function normalise(clientX: number, clientY: number, rect: DOMRect): [number, number] {
  const clamp = (v: number) => Math.round(Math.min(1, Math.max(0, v)) * 10_000) / 10_000;
  return [clamp((clientX - rect.left) / rect.width), clamp((clientY - rect.top) / rect.height)];
}
