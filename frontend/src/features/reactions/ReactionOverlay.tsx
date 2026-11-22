import { useFloatingReactions } from './useReactions';

/** Emojis floating up over the stage. */
export function ReactionOverlay() {
  const items = useFloatingReactions((s) => s.items);
  return (
    <div className="pointer-events-none absolute inset-0 z-10 overflow-hidden" aria-live="polite">
      {items.map((r) => (
        <div
          key={r.id}
          className="reaction-float absolute bottom-6 flex flex-col items-center gap-1"
          style={{ left: `${r.left}%` }}
        >
          <span className="text-4xl drop-shadow md:text-5xl" role="img" aria-label={r.emoji}>
            {r.emoji}
          </span>
          <span className="max-w-28 truncate rounded bg-black/60 px-1.5 text-xs text-white">
            {r.name}
          </span>
        </div>
      ))}
    </div>
  );
}
