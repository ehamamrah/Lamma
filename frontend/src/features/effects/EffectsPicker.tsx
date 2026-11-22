import { Ban, ImagePlus } from 'lucide-react';
import { type ChangeEvent, type ReactNode, useState } from 'react';
import { cn } from '@/shared/lib/utils';
import { BACKGROUNDS, MASKS, effectsSupported, useEffectsStore } from './effects';
import { SCENE_SWATCH } from './scenes';
import { useVisionStatus } from './vision';

const MAX_IMAGE_BYTES = 15 * 1024 * 1024;
const MAX_IMAGE_PIXELS = 40_000_000; // decoded size; a tiny file can be huge
const KEEP_WIDTH = 1920;

/** Decodes an image, refusing huge ones, and keeps at most 1920 px wide. */
async function decode(file: File): Promise<ImageBitmap> {
  const full = await createImageBitmap(file);
  if (full.width * full.height > MAX_IMAGE_PIXELS) {
    full.close();
    throw new Error('too large');
  }
  if (full.width <= KEEP_WIDTH) return full;
  const small = await createImageBitmap(full, { resizeWidth: KEEP_WIDTH, resizeQuality: 'high' });
  full.close();
  return small;
}

/** Background and face-mask choices for the local camera. */
export function EffectsPicker({ className }: { className?: string }) {
  if (!effectsSupported()) {
    return (
      <p className={cn('text-sm text-muted-foreground', className)}>
        Camera effects aren&apos;t supported in this browser.
      </p>
    );
  }
  return (
    <div className={cn('flex flex-col gap-4', className)}>
      <Backgrounds />
      <Masks />
      <Status />
    </div>
  );
}

function Backgrounds() {
  const background = useEffectsStore((s) => s.background);
  const setBackground = useEffectsStore((s) => s.setBackground);
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-2 text-sm font-medium">Background</legend>
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-5">
        {BACKGROUNDS.filter((b) => b.id !== 'custom').map((b) => (
          <Tile
            key={b.id}
            label={b.label}
            pressed={background === b.id}
            onClick={() => setBackground(b.id)}
            swatch={SCENE_SWATCH[b.id]}
          >
            {b.id === 'none' ? <Ban className="size-4" /> : null}
          </Tile>
        ))}
        <Upload pressed={background === 'custom'} />
      </div>
    </fieldset>
  );
}

/** A background picture from this device. It is never uploaded anywhere. */
function Upload({ pressed }: { pressed: boolean }) {
  const setCustomImage = useEffectsStore((s) => s.setCustomImage);
  const [error, setError] = useState<string | null>(null);

  async function onFile(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    setError(null);
    if (!file.type.startsWith('image/') || file.size > MAX_IMAGE_BYTES) {
      setError('Choose an image file under 15 MB.');
      return;
    }
    try {
      setCustomImage(await decode(file));
    } catch {
      setError('That image could not be opened, or is too large.');
    }
  }

  return (
    <>
      <label
        className={cn(
          'flex h-14 cursor-pointer flex-col items-center justify-center gap-1 rounded-lg border border-border text-xs focus-within:ring-2 focus-within:ring-ring',
          pressed && 'ring-2 ring-primary',
        )}
      >
        <ImagePlus className="size-4" />
        Your image
        <input type="file" accept="image/*" className="sr-only" onChange={onFile} />
      </label>
      {error && <p className="col-span-full text-sm text-destructive">{error}</p>}
    </>
  );
}

function Masks() {
  const mask = useEffectsStore((s) => s.mask);
  const setMask = useEffectsStore((s) => s.setMask);
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-2 text-sm font-medium">Face mask</legend>
      <div className="grid grid-cols-4 gap-2 sm:grid-cols-7">
        {MASKS.map((m) => (
          <Tile key={m.id} label={m.label} pressed={mask === m.id} onClick={() => setMask(m.id)}>
            <span className="text-xl" aria-hidden>
              {m.emoji}
            </span>
          </Tile>
        ))}
      </div>
    </fieldset>
  );
}

function Tile(props: {
  label: string;
  pressed: boolean;
  onClick: () => void;
  swatch?: string;
  children?: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-pressed={props.pressed}
      onClick={props.onClick}
      style={props.swatch ? { background: props.swatch } : undefined}
      className={cn(
        'flex h-14 flex-col items-center justify-center gap-0.5 rounded-lg border border-border bg-muted text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        props.swatch && 'text-white [text-shadow:0_1px_2px_rgb(0_0_0/0.7)]',
        props.pressed && 'ring-2 ring-primary',
      )}
    >
      {props.children}
      {props.label}
    </button>
  );
}

function Status() {
  const { segmenter, face } = useVisionStatus();
  const text =
    segmenter === 'error' || face === 'error'
      ? 'Some effects could not load on this device.'
      : segmenter === 'loading' || face === 'loading'
        ? 'Loading effects…'
        : null;
  return text ? (
    <p className="text-sm text-muted-foreground" role="status">
      {text}
    </p>
  ) : null;
}
