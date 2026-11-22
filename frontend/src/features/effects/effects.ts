import { create } from 'zustand';

export const BACKGROUNDS = [
  { id: 'none', label: 'None' },
  { id: 'blur', label: 'Light blur' },
  { id: 'blur-strong', label: 'Strong blur' },
  { id: 'studio', label: 'Studio' },
  { id: 'sunset', label: 'Sunset' },
  { id: 'ocean', label: 'Ocean' },
  { id: 'forest', label: 'Forest' },
  { id: 'night', label: 'Night sky' },
  { id: 'custom', label: 'Your image' },
] as const;

export const MASKS = [
  { id: 'none', label: 'None', emoji: '🚫' },
  { id: 'sunglasses', label: 'Sunglasses', emoji: '🕶️' },
  { id: 'cat', label: 'Cat', emoji: '🐱' },
  { id: 'party', label: 'Party hat', emoji: '🥳' },
  { id: 'mustache', label: 'Mustache', emoji: '🥸' },
  { id: 'clown', label: 'Clown', emoji: '🤡' },
  { id: 'crown', label: 'Crown', emoji: '👑' },
] as const;

export type BackgroundId = (typeof BACKGROUNDS)[number]['id'];
export type MaskId = (typeof MASKS)[number]['id'];

export interface EffectSettings {
  background: BackgroundId;
  mask: MaskId;
}

interface EffectsState extends EffectSettings {
  /** The uploaded background. It stays in this browser tab, never uploaded. */
  customImage: ImageBitmap | null;
  setBackground: (b: BackgroundId) => void;
  setMask: (m: MaskId) => void;
  setCustomImage: (img: ImageBitmap) => void;
}

const KEY = 'lamma.effects';

/** Reads saved choices, ignoring anything unknown. */
export function loadSettings(raw: string | null): EffectSettings {
  const fallback: EffectSettings = { background: 'none', mask: 'none' };
  try {
    const v = JSON.parse(raw ?? '{}') as Partial<EffectSettings>;
    const bg = BACKGROUNDS.find((b) => b.id === v.background && b.id !== 'custom');
    const mask = MASKS.find((m) => m.id === v.mask);
    return { background: bg?.id ?? 'none', mask: mask?.id ?? 'none' };
  } catch {
    return fallback;
  }
}

/** Storage can be blocked by privacy settings; effects then just aren't remembered. */
function storage(): Storage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

function read(): EffectSettings {
  try {
    return loadSettings(storage()?.getItem(KEY) ?? null);
  } catch {
    return loadSettings(null);
  }
}

export const useEffectsStore = create<EffectsState>((set) => ({
  ...read(),
  customImage: null,
  setBackground: (background) => set({ background }),
  setMask: (mask) => set({ mask }),
  setCustomImage: (customImage) =>
    set((s) => {
      s.customImage?.close();
      return { customImage, background: 'custom' };
    }),
}));

// Remember the choice for next time (the custom image is not saved).
useEffectsStore.subscribe((s, prev) => {
  if (s.background === prev.background && s.mask === prev.mask) return;
  const background = s.background === 'custom' ? 'none' : s.background;
  try {
    storage()?.setItem(KEY, JSON.stringify({ background, mask: s.mask }));
  } catch {
    // Full or blocked storage: the choice still applies to this visit.
  }
});

export const isActive = (s: EffectSettings) => s.background !== 'none' || s.mask !== 'none';

/** Effects need a canvas we can stream, WebAssembly, and workers with OffscreenCanvas. */
export const effectsSupported = () =>
  typeof WebAssembly === 'object' &&
  typeof Worker === 'function' &&
  typeof OffscreenCanvas === 'function' &&
  typeof HTMLCanvasElement !== 'undefined' &&
  typeof HTMLCanvasElement.prototype.captureStream === 'function';
