import { SmilePlus } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@/shared/ui/button';
import { EMOJIS, type Emoji } from './reactions';

/** Control-bar button that opens a row of emojis. */
export function ReactionPicker({ onReact }: { onReact: (e: Emoji) => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('pointerdown', onDown);
    return () => document.removeEventListener('pointerdown', onDown);
  }, [open]);

  return (
    <div ref={ref} className="md:relative">
      <Button
        variant={open ? 'default' : 'secondary'}
        size="icon"
        aria-label="Reactions"
        aria-expanded={open}
        title="Reactions"
        onClick={() => setOpen(!open)}
      >
        <SmilePlus />
      </Button>
      {open && (
        <div
          role="menu"
          aria-label="Send a reaction"
          className="absolute inset-x-2 bottom-full z-30 mb-2 flex justify-center gap-1 rounded-full border border-border bg-card p-1 shadow-lg md:inset-x-auto md:left-1/2 md:-translate-x-1/2"
        >
          {EMOJIS.map((e) => (
            <button
              key={e}
              type="button"
              role="menuitem"
              aria-label={`React ${e}`}
              className="rounded-full p-1.5 text-2xl transition-transform hover:scale-125 hover:bg-muted active:scale-110"
              onClick={() => onReact(e)}
            >
              {e}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
