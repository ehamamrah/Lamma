import type { Track, TrackProcessor, VideoProcessorOptions } from 'livekit-client';
import { type EffectSettings, useEffectsStore } from './effects';
import { faceGeometry } from './geometry';
import { drawMask } from './masks';
import { SceneCache, type SceneId } from './scenes';
import { analyse } from './vision';
import type { Landmark, PersonMask } from './visionTypes';

const FPS = 30;
const MAX_W = 1280;
const PRIVACY_BLUR = 40;
/**
 * Effects may use at most 1/DUTY of the main thread: after a slow frame the
 * next ticks are skipped, so weak devices get fewer frames instead of a
 * frozen page.
 */
const DUTY = 3;
/** Width of the frame sent for analysis (enough for faces at a distance). */
const ANALYSIS_W = 640;

/** What the processor draws; read every frame so changes apply instantly. */
export interface EffectsInput extends EffectSettings {
  customImage: ImageBitmap | null;
}

/**
 * A LiveKit video processor: it draws each camera frame onto a canvas with
 * the chosen background and face mask, and LiveKit publishes the canvas
 * instead of the camera. Everything runs in this browser. While a background
 * is selected, no raw frame is drawn: until the model is ready, or if a frame
 * fails, a heavy blur is sent instead.
 *
 * The models run in a worker (vision.worker.ts), one frame at a time, so
 * their speed adapts to the device and never blocks the page. Every frame
 * is composited here with the latest mask and landmarks.
 */
export class EffectsProcessor implements TrackProcessor<Track.Kind.Video> {
  readonly name = 'lamma-effects';
  processedTrack?: MediaStreamTrack;

  private own = document.createElement('video');
  private video = this.own;
  private canvas = document.createElement('canvas');
  private ctx = this.canvas.getContext('2d')!;
  private mask = new MaskBuffer();
  private faces: Landmark[][] = [];
  private analysing = false;
  private generation = 0; // bumped on restart, so stale results are dropped
  private blur = new Blur();
  private scenes = new SceneCache();
  private clock = new Clock(() => this.safeFrame());
  private warned = false;
  private busyUntil = 0;
  private lastFrameTime = -1;

  constructor(private input: () => EffectsInput = () => useEffectsStore.getState()) {}

  async init({ track, element }: VideoProcessorOptions) {
    // LiveKit hands us a muted element already playing the camera; reuse it
    // rather than decoding the same track twice.
    if (element instanceof HTMLVideoElement) this.video = element;
    else {
      this.video = this.own;
      this.own.muted = true;
      this.own.playsInline = true;
      this.own.srcObject = new MediaStream([track]);
      await this.own.play().catch(() => undefined);
    }
    this.processedTrack ??= this.canvas.captureStream(FPS).getVideoTracks()[0];
    this.clock.start(1000 / FPS);
  }

  /** LiveKit calls this when the camera track is replaced (unmute, new device). */
  async restart(opts: VideoProcessorOptions) {
    this.clock.stop();
    this.generation++;
    this.mask.ready = false; // blur until the new camera has been segmented
    this.faces = [];
    await this.init(opts);
  }

  async destroy() {
    this.clock.dispose();
    this.generation++;
    this.processedTrack?.stop();
    this.processedTrack = undefined;
    this.own.srcObject = null;
    for (const c of [this.canvas, this.mask.canvas]) c.width = c.height = 0;
    this.scenes.clear();
  }

  private safeFrame() {
    const start = performance.now();
    if (start < this.busyUntil) return;
    this.drawFrame();
    this.busyUntil = start + (performance.now() - start) * DUTY;
  }

  private drawFrame() {
    const s = this.input();
    try {
      this.frame(s);
    } catch (err) {
      if (!this.warned) console.warn('camera effects: frame failed', err);
      this.warned = true;
      const { width: w, height: h } = this.canvas;
      if (s.background !== 'none') this.blur.draw(this.ctx, this.video, w, h, PRIVACY_BLUR);
      else this.ctx.drawImage(this.video, 0, 0, w, h);
    }
  }

  private frame(s: EffectsInput) {
    const v = this.video;
    if (v.readyState < 2 || v.currentTime === this.lastFrameTime) return;
    this.lastFrameTime = v.currentTime;
    const { w, h } = this.fit(v.videoWidth, v.videoHeight);
    this.requestAnalysis(s);
    if (s.background === 'none') this.ctx.drawImage(v, 0, 0, w, h);
    else this.composite(s, w, h);
    if (s.mask !== 'none') this.drawMasks(s, w, h);
  }

  private fit(vw: number, vh: number) {
    const k = Math.min(1, MAX_W / vw);
    const w = Math.round(vw * k);
    const h = Math.round(vh * k);
    if (this.canvas.width !== w || this.canvas.height !== h) {
      this.canvas.width = w;
      this.canvas.height = h;
    }
    return { w, h };
  }

  /** Sends the current frame to the worker unless one is still being analysed. */
  private requestAnalysis(s: EffectsInput) {
    if (this.analysing) return;
    const want = { segment: s.background !== 'none', faces: s.mask !== 'none' };
    const v = this.video;
    const gen = this.generation;
    this.analysing = true;
    createImageBitmap(v, {
      resizeWidth: ANALYSIS_W,
      resizeHeight: Math.round((ANALYSIS_W * v.videoHeight) / v.videoWidth),
    })
      .then((bitmap) => analyse(bitmap, want))
      .then((r) => {
        if (gen !== this.generation) return;
        if (r.mask) this.mask.write(r.mask);
        if (want.faces) this.faces = r.faces ?? this.faces;
      })
      .catch(() => undefined) // a skipped frame; the next tick tries again
      .finally(() => (this.analysing = false));
  }

  private composite(s: EffectsInput, w: number, h: number) {
    const ctx = this.ctx;
    if (!this.mask.ready) {
      // Model still loading: send a heavy blur rather than the real room.
      this.blur.draw(ctx, this.video, w, h, PRIVACY_BLUR);
      return;
    }
    ctx.save();
    try {
      this.cutOut(s, w, h);
    } finally {
      ctx.restore();
    }
  }

  /** The person over the chosen background, using the latest mask. */
  private cutOut(s: EffectsInput, w: number, h: number) {
    const ctx = this.ctx;
    ctx.globalCompositeOperation = 'copy';
    ctx.drawImage(this.mask.canvas, 0, 0, w, h); // alpha = "this is a person"
    ctx.globalCompositeOperation = 'source-in';
    ctx.drawImage(this.video, 0, 0, w, h); // keep the person only
    ctx.globalCompositeOperation = 'destination-over'; // fill in behind them
    if (s.background === 'blur' || s.background === 'blur-strong') {
      this.blur.draw(ctx, this.video, w, h, s.background === 'blur' ? 10 : 24);
    } else {
      ctx.drawImage(this.scenes.get(s.background as SceneId, w, h, s.customImage), 0, 0);
    }
  }

  private drawMasks(s: EffectsInput, w: number, h: number) {
    for (const lm of this.faces) {
      const face = faceGeometry(lm, w, h);
      if (face) drawMask(this.ctx, s.mask, face);
    }
  }
}

/** The segmentation result as a canvas whose alpha is "person". */
class MaskBuffer {
  canvas = document.createElement('canvas');
  ready = false;
  private ctx = this.canvas.getContext('2d')!;
  private img: ImageData | null = null;

  write(m: PersonMask) {
    if (!this.img || this.img.width !== m.width || this.img.height !== m.height) {
      this.canvas.width = m.width;
      this.canvas.height = m.height;
      this.img = this.ctx.createImageData(m.width, m.height);
      this.img.data.fill(255);
    }
    const conf = m.data;
    const px = this.img.data;
    for (let i = 0; i < conf.length; i++) {
      // Soft threshold: crisper edges without a hard cut-out look.
      const a = Math.min(1, Math.max(0, ((conf[i] ?? 0) - 0.25) * 2));
      px[i * 4 + 3] = a * 255;
    }
    this.ctx.putImageData(this.img, 0, 0);
    this.ready = true;
  }
}

const BLUR_SCALE = 4;

/**
 * Blurs a frame cheaply: a quarter-size copy is blurred (with a canvas
 * filter where supported) and scaled back up. Blurring at full size is far
 * too slow without a GPU.
 */
class Blur {
  private small = document.createElement('canvas');
  private sctx = this.small.getContext('2d')!;
  private filter: boolean | null = null;

  draw(ctx: CanvasRenderingContext2D, src: CanvasImageSource, w: number, h: number, px: number) {
    this.filter ??= supportsFilter(this.sctx);
    // Without filters, shrink further so the upscale alone blurs enough.
    const k = this.filter ? BLUR_SCALE : Math.max(BLUR_SCALE, px / 2);
    const sw = Math.max(8, Math.round(w / k));
    const sh = Math.max(8, Math.round(h / k));
    if (this.small.width !== sw || this.small.height !== sh) {
      this.small.width = sw;
      this.small.height = sh;
    }
    if (this.filter) {
      const r = px / k;
      this.sctx.filter = `blur(${r}px)`;
      this.sctx.drawImage(src, -r, -r, sw + 2 * r, sh + 2 * r); // hide soft edges
      this.sctx.filter = 'none';
    } else this.sctx.drawImage(src, 0, 0, sw, sh);
    ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(this.small, 0, 0, w, h);
  }
}

function supportsFilter(ctx: CanvasRenderingContext2D) {
  if (!('filter' in ctx)) return false;
  const before = ctx.filter;
  ctx.filter = 'blur(1px)';
  const ok = ctx.filter === 'blur(1px)';
  ctx.filter = before;
  return ok;
}

/**
 * Calls `tick` at a steady rate. A worker's timers keep running when the tab
 * is in the background (requestAnimationFrame does not); if the worker
 * cannot start, a plain interval takes over.
 */
class Clock {
  private worker: Worker | null = null;
  private timer: ReturnType<typeof setInterval> | undefined;

  constructor(private tick: () => void) {}

  start(ms: number) {
    this.stop();
    const fallback = () => {
      this.worker?.terminate();
      this.worker = null;
      clearInterval(this.timer);
      this.timer = setInterval(this.tick, ms);
    };
    try {
      this.worker ??= new Worker(new URL('./ticker.worker.ts', import.meta.url), {
        type: 'module',
      });
      this.worker.onmessage = () => this.tick();
      this.worker.onerror = fallback; // load failures are reported here, not thrown
      this.worker.postMessage(ms);
    } catch {
      fallback();
    }
  }

  stop() {
    this.worker?.postMessage(0);
    clearInterval(this.timer);
  }

  dispose() {
    this.stop();
    this.worker?.terminate();
    this.worker = null;
  }
}
