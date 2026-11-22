import type { MaskId } from './effects';
import type { Face, Pt } from './geometry';

type Ctx = CanvasRenderingContext2D;
type Draw = (ctx: Ctx, f: Face) => void;

/**
 * Runs `draw` in face space: origin at `at`, x along the eyes, y down the
 * face, rotated with the head. Sizes are fractions of the face width `u`.
 */
function inFace(ctx: Ctx, f: Face, at: Pt, draw: (u: number) => void) {
  ctx.save();
  ctx.translate(at.x, at.y);
  ctx.rotate(f.angle);
  draw(f.width);
  ctx.restore();
}

const sunglasses: Draw = (ctx, f) =>
  inFace(ctx, f, f.eyeMid, (u) => {
    const d = f.eyeDist;
    const lw = d * 0.9;
    const lh = d * 0.6;
    ctx.lineWidth = u * 0.025;
    ctx.strokeStyle = '#111';
    ctx.beginPath(); // arms and bridge
    ctx.moveTo(-u * 0.5, -lh * 0.15);
    ctx.lineTo(-d / 2 - lw / 2, -lh * 0.2);
    ctx.moveTo(-d / 2 + lw / 2, -lh * 0.2);
    ctx.quadraticCurveTo(0, -lh * 0.45, d / 2 - lw / 2, -lh * 0.2);
    ctx.moveTo(d / 2 + lw / 2, -lh * 0.2);
    ctx.lineTo(u * 0.5, -lh * 0.15);
    ctx.stroke();
    for (const cx of [-d / 2, d / 2]) {
      const g = ctx.createLinearGradient(0, -lh / 2, 0, lh / 2);
      g.addColorStop(0, '#3a3f4a');
      g.addColorStop(1, '#0b0c10');
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.roundRect(cx - lw / 2, -lh / 2, lw, lh, lh * 0.45);
      ctx.fill();
      ctx.stroke();
      ctx.fillStyle = 'rgba(255,255,255,0.25)'; // glare
      ctx.beginPath();
      ctx.ellipse(cx - lw * 0.2, -lh * 0.2, lw * 0.15, lh * 0.08, -0.4, 0, Math.PI * 2);
      ctx.fill();
    }
  });

const cat: Draw = (ctx, f) => {
  inFace(ctx, f, f.top, (u) => {
    for (const s of [-1, 1]) {
      ear(ctx, s * u * 0.3, u, '#4a3b35', 1);
      ear(ctx, s * u * 0.3, u, '#f4a6b8', 0.55);
    }
  });
  inFace(ctx, f, f.nose, (u) => {
    ctx.fillStyle = '#f07b9a';
    ctx.beginPath();
    ctx.moveTo(-u * 0.06, -u * 0.03);
    ctx.lineTo(u * 0.06, -u * 0.03);
    ctx.lineTo(0, u * 0.04);
    ctx.closePath();
    ctx.fill();
    ctx.strokeStyle = 'rgba(30,30,30,0.85)';
    ctx.lineWidth = u * 0.008;
    ctx.beginPath();
    for (const s of [-1, 1]) {
      for (const dy of [-0.04, 0.01, 0.06]) {
        ctx.moveTo(s * u * 0.1, u * 0.03);
        ctx.lineTo(s * u * 0.42, u * (0.03 + dy * 1.6));
      }
    }
    ctx.stroke();
  });
};

function ear(ctx: Ctx, cx: number, u: number, color: string, k: number) {
  ctx.fillStyle = color;
  ctx.beginPath();
  ctx.moveTo(cx - u * 0.15 * k, -u * 0.04);
  ctx.lineTo(cx + Math.sign(cx) * u * 0.04 * k, -u * (0.04 + 0.38 * k));
  ctx.lineTo(cx + u * 0.15 * k, -u * 0.04);
  ctx.closePath();
  ctx.fill();
}

const party: Draw = (ctx, f) =>
  inFace(ctx, f, f.top, (u) => {
    const base = -u * 0.1;
    const tip = -u * 0.76;
    ctx.save();
    ctx.beginPath();
    ctx.moveTo(-u * 0.26, base);
    ctx.lineTo(0, tip);
    ctx.lineTo(u * 0.26, base);
    ctx.closePath();
    ctx.fillStyle = '#7c4dff';
    ctx.fill();
    ctx.clip();
    const colors = ['#ff4081', '#ffd740', '#18ffff'];
    for (let i = 0; i < 6; i++) {
      ctx.fillStyle = colors[i % colors.length] ?? '#fff';
      const y = base - (i + 0.5) * u * 0.11;
      ctx.fillRect(-u * 0.3, y, u * 0.6, u * 0.04);
    }
    ctx.restore();
    ctx.fillStyle = '#ffd740';
    ctx.beginPath();
    ctx.arc(0, tip, u * 0.06, 0, Math.PI * 2);
    ctx.fill();
  });

const mustache: Draw = (ctx, f) =>
  inFace(ctx, f, f.underNose, (u) => {
    ctx.fillStyle = '#3b2314';
    ctx.beginPath();
    ctx.moveTo(0, -u * 0.025);
    for (const s of [1, -1]) {
      ctx.bezierCurveTo(s * u * 0.12, -u * 0.08, s * u * 0.24, -u * 0.02, s * u * 0.3, -u * 0.09);
      ctx.bezierCurveTo(s * u * 0.27, u * 0.04, s * u * 0.12, u * 0.05, 0, u * 0.015);
    }
    ctx.fill();
  });

const clown: Draw = (ctx, f) => {
  for (const c of f.cheeks) {
    inFace(ctx, f, c, (u) => {
      ctx.fillStyle = 'rgba(255, 64, 96, 0.45)';
      ctx.beginPath();
      ctx.arc(0, 0, u * 0.1, 0, Math.PI * 2);
      ctx.fill();
    });
  }
  inFace(ctx, f, f.nose, (u) => {
    const r = u * 0.11;
    const g = ctx.createRadialGradient(-r * 0.35, -r * 0.35, r * 0.1, 0, 0, r);
    g.addColorStop(0, '#ff8a80');
    g.addColorStop(1, '#d50000');
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(0, 0, r, 0, Math.PI * 2);
    ctx.fill();
  });
};

const crown: Draw = (ctx, f) =>
  inFace(ctx, f, f.top, (u) => {
    const w = u * 0.62;
    const base = -u * 0.08;
    const h = u * 0.32;
    const g = ctx.createLinearGradient(0, base - h, 0, base);
    g.addColorStop(0, '#ffe57f');
    g.addColorStop(1, '#c79100');
    ctx.fillStyle = g;
    ctx.strokeStyle = '#8d6e00';
    ctx.lineWidth = u * 0.01;
    ctx.beginPath();
    ctx.moveTo(-w / 2, base);
    const peaks = 5;
    for (let i = 0; i <= peaks * 2; i++) {
      const x = -w / 2 + (w * i) / (peaks * 2);
      const y = i % 2 === 0 ? base - h : base - h * 0.45;
      ctx.lineTo(x, y);
    }
    ctx.lineTo(w / 2, base);
    ctx.closePath();
    ctx.fill();
    ctx.stroke();
    const jewels = ['#e53935', '#1e88e5', '#43a047', '#1e88e5', '#e53935'];
    jewels.forEach((c, i) => {
      ctx.fillStyle = c;
      ctx.beginPath();
      ctx.arc(-w / 2 + (w * (i + 0.5)) / peaks, base - h * 0.2, u * 0.025, 0, Math.PI * 2);
      ctx.fill();
    });
  });

const DRAW: Record<Exclude<MaskId, 'none'>, Draw> = {
  sunglasses,
  cat,
  party,
  mustache,
  clown,
  crown,
};

export function drawMask(ctx: Ctx, id: MaskId, face: Face) {
  if (id !== 'none') DRAW[id](ctx, face);
}
