import type { ReactNode } from 'react';
import { Button } from '@/shared/ui/button';
import { cn } from '@/shared/lib/utils';

/** A control-bar action: an icon button on desktop, a row in the mobile sheet. */
export interface Action {
  key: string;
  label: string;
  icon: ReactNode;
  onClick: () => void;
  active?: boolean;
  badge?: number;
  danger?: boolean;
  /** Show the label next to the icon on desktop. */
  wide?: boolean;
}

export function ActionButton({ action: a }: { action: Action }) {
  return (
    <span className="relative">
      <Button
        variant={a.danger ? 'destructive' : a.active ? 'default' : 'secondary'}
        size={a.wide ? 'default' : 'icon'}
        aria-label={a.label}
        aria-pressed={a.active}
        title={a.label}
        onClick={a.onClick}
      >
        {a.icon}
        {a.wide && a.label}
      </Button>
      <Badge count={a.badge} />
    </span>
  );
}

export function ActionRow({ action: a, onDone }: { action: Action; onDone: () => void }) {
  return (
    <button
      type="button"
      aria-pressed={a.active}
      className={cn(
        'flex w-full items-center gap-3 rounded-lg px-3 py-3 text-left text-sm active:bg-muted [&_svg]:size-5',
        a.active && 'text-primary',
        a.danger && 'text-destructive',
      )}
      onClick={() => {
        a.onClick();
        onDone();
      }}
    >
      {a.icon}
      <span className="flex-1">{a.label}</span>
      {!!a.badge && (
        <span className="rounded-full bg-destructive px-2 text-xs text-destructive-foreground">
          {a.badge}
        </span>
      )}
    </button>
  );
}

function Badge({ count }: { count?: number }) {
  if (!count) return null;
  return (
    <span
      className="pointer-events-none absolute -right-1 -top-1 rounded-full bg-destructive px-1.5 text-xs text-destructive-foreground"
      aria-label={`${count} pending`}
    >
      {count}
    </span>
  );
}

/** Bottom sheet with the secondary actions (phones). */
export function MoreSheet({ actions, onClose }: { actions: Action[]; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-40 flex items-end bg-black/50" onClick={onClose}>
      <div
        role="dialog"
        aria-label="More options"
        className="w-full rounded-t-2xl border-t border-border bg-card p-2 pb-[max(0.5rem,env(safe-area-inset-bottom))]"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mx-auto mb-2 h-1 w-10 rounded-full bg-border" aria-hidden />
        {actions.map((a) => (
          <ActionRow key={a.key} action={a} onDone={onClose} />
        ))}
      </div>
    </div>
  );
}
