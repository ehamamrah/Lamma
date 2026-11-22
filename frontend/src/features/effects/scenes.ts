import type { BackgroundId } from './effects';

type Ctx = CanvasRenderingContext2D;
export type SceneId = Exclude<BackgroundId, 'none' | 'blur' | 'blur-strong'>;

/** CSS previews for the picker, matching the painted scenes. */
export const SCENE_SWATCH: Partial<Record<BackgroundId, string>> = {
  studio: 'radial-gradient(circle at 50% 40%, #f2f2f2, #8a8f98)',
  sunset: 'linear-gradient(#ff7e5f, #feb47b 60%, #ffd194)',
  ocean: 'linear-gradient(#89d4f5, #2b88c7 55%, #0b3d63)',
  forest: 'linear-gradient(#cfe8c4, #4f8a4a 60%, #1f3d1d)',
  night: 'linear-gradient(#0b1026, #1c2a5a)',
};

function linear(ctx: Ctx, h: number, stops: [number, string][]) {
  const g = ctx.createLinearGradient(0, 0, 0, h);
  for (const [at, c] of stops) g.addColorStop(at, c);
  return g;
}

/** Deterministic pseudo-random numbers, so scenes look the same each frame. */
function seeded(seed: number) {
  return () => {
    seed = (seed * 1664525 + 1013904223) % 4294967296;
    return seed / 4294967296;
  };
}

const PAINT: Record<Exclude<SceneId, 'custom'>, (ctx: Ctx, w: number, h: number) => void> = {
  studio(ctx, w, h) {
    const g = ctx.createRadialGradient(w / 2, h * 0.4, 0, w / 2, h * 0.4, Math.max(w, h) * 0.7);
    g.addColorStop(0, '#f2f2f2');
    g.addColorStop(1, '#8a8f98');
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, w, h);
  },
  sunset(ctx, w, h) {
    ctx.fillStyle = linear(ctx, h, [
      [0, '#ff7e5f'],
      [0.6, '#feb47b'],
      [1, '#ffd194'],
    ]);
    ctx.fillRect(0, 0, w, h);
    ctx.fillStyle = 'rgba(255, 245, 200, 0.85)';
    ctx.beginPath();
    ctx.arc(w * 0.75, h * 0.62, h * 0.16, 0, Math.PI * 2);
    ctx.fill();
  },
  ocean(ctx, w, h) {
    ctx.fillStyle = linear(ctx, h, [
      [0, '#89d4f5'],
      [0.55, '#bfe7f7'],
      [0.56, '#2b88c7'],
      [1, '#0b3d63'],
    ]);
    ctx.fillRect(0, 0, w, h);
  },
  forest(ctx, w, h) {
    ctx.fillStyle = linear(ctx, h, [
      [0, '#cfe8c4'],
      [1, '#7fb174'],
    ]);
    ctx.fillRect(0, 0, w, h);
    const rnd = seeded(7);
    for (const [shade, base, size] of [
      ['#4f8a4a', 0.75, 0.35],
      ['#1f3d1d', 1, 0.5],
    ] as const) {
      ctx.fillStyle = shade;
      for (let x = -0.05; x < 1.05; x += 0.07 + rnd() * 0.05) {
        const th = h * size * (0.7 + rnd() * 0.5);
        ctx.beginPath();
        ctx.moveTo(w * x - th * 0.3, h * base);
        ctx.lineTo(w * x, h * base - th);
        ctx.lineTo(w * x + th * 0.3, h * base);
        ctx.fill();
      }
    }
  },
  night(ctx, w, h) {
    ctx.fillStyle = linear(ctx, h, [
      [0, '#0b1026'],
      [1, '#1c2a5a'],
    ]);
    ctx.fillRect(0, 0, w, h);
    const rnd = seeded(42);
    ctx.fillStyle = '#fff';
    for (let i = 0; i < 160; i++) {
      ctx.globalAlpha = 0.3 + rnd() * 0.7;
      ctx.beginPath();
      ctx.arc(rnd() * w, rnd() * h, 0.5 + rnd() * 1.5, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1;
  },
};

/** Draws `img` to fill w×h, cropping the overflow (CSS "cover"). */
function cover(ctx: Ctx, img: ImageBitmap, w: number, h: number) {
  const s = Math.max(w / img.width, h / img.height);
  const dw = img.width * s;
  const dh = img.height * s;
  ctx.drawImage(img, (w - dw) / 2, (h - dh) / 2, dw, dh);
}

/** Paints scenes once per size and choice; every frame reuses the result. */
export class SceneCache {
  private canvas = document.createElement('canvas');
  private key = '';
  private image: ImageBitmap | null = null;

  clear() {
    this.canvas.width = this.canvas.height = 0;
    this.key = '';
    this.image = null;
  }

  get(id: SceneId, w: number, h: number, custom: ImageBitmap | null): HTMLCanvasElement {
    const key = `${id}:${w}x${h}`;
    if (key === this.key && (id !== 'custom' || custom === this.image)) return this.canvas;
    this.key = key;
    this.image = custom;
    this.canvas.width = w;
    this.canvas.height = h;
    const ctx = this.canvas.getContext('2d')!;
    if (id === 'custom') {
      if (custom) cover(ctx, custom, w, h);
      else PAINT.studio(ctx, w, h);
    } else PAINT[id](ctx, w, h);
    return this.canvas;
  }
}
