import { useLocalParticipant, useRoomContext } from '@livekit/components-react';
import { Hand, Pencil, Trash2, Undo2, X } from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/shared/ui/button';
import { cn } from '@/shared/lib/utils';
import { boardHost, useCollabStore } from '@/stores/collab';
import { useMeetingStore } from '@/stores/meeting';
import { publish } from '@/features/room/data';
import { hostIdentities } from '@/features/room/participant';
import { BoardCanvas } from './BoardCanvas';
import { DrawRequests } from './DrawRequests';
import { type BoardMode, COLORS, TOPIC, WIDTHS, canDraw } from '@/features/collab/protocol';
import { useBoardOp } from './useBoardDrawing';

/** The shared whiteboard: toolbar, permissions (host) and canvas. */
export function Whiteboard() {
  const amHost = useMeetingStore((s) => s.me?.role === 'host'); // UI only; peers verify
  const me = useLocalParticipant().localParticipant.identity;
  const host = useCollabStore((s) => s.host);
  const allowed = canDraw(host, me, amHost);
  const [color, setColor] = useState<string>(COLORS[0]);
  const [width, setWidth] = useState<number>(WIDTHS[1]);

  return (
    <section className="flex h-full flex-col gap-2" aria-label="Whiteboard">
      <div className="flex flex-wrap items-center gap-2">
        {allowed && <PenTools color={color} width={width} onColor={setColor} onWidth={setWidth} />}
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {amHost ? <HostTools mode={host.board} /> : <GuestStatus me={me} />}
        </div>
      </div>
      {amHost && <DrawRequests />}
      <BoardCanvas canDraw={allowed} color={color} width={width} />
    </section>
  );
}

function PenTools(props: {
  color: string;
  width: number;
  onColor: (c: string) => void;
  onWidth: (w: number) => void;
}) {
  const op = useBoardOp();
  const me = useLocalParticipant().localParticipant.identity;
  const undo = () => {
    const mine = useCollabStore.getState().strokes.findLast((s) => s.by === me);
    if (mine) op({ t: 'undo', id: mine.id });
  };
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="toolbar" aria-label="Pen">
      {COLORS.map((c) => (
        <button
          key={c}
          type="button"
          aria-label={`Colour ${c}`}
          aria-pressed={props.color === c}
          className={cn(
            'size-7 rounded-full border-2 border-transparent',
            props.color === c && 'border-white ring-2 ring-primary',
          )}
          style={{ background: c }}
          onClick={() => props.onColor(c)}
        />
      ))}
      {WIDTHS.map((w) => (
        <Button
          key={w}
          size="icon"
          variant={props.width === w ? 'default' : 'secondary'}
          className="size-8"
          aria-label={`Pen size ${w}`}
          onClick={() => props.onWidth(w)}
        >
          <span className="rounded-full bg-current" style={{ width: w + 2, height: w + 2 }} />
        </Button>
      ))}
      <Button
        size="icon"
        variant="secondary"
        className="size-8"
        aria-label="Undo my last line"
        title="Undo"
        onClick={undo}
      >
        <Undo2 />
      </Button>
    </div>
  );
}

function HostTools({ mode }: { mode: BoardMode }) {
  const op = useBoardOp();
  const modes: [BoardMode, string][] = [
    ['solo', 'Only me'],
    ['collab', 'Collaborative'],
  ];
  return (
    <>
      <div className="flex rounded-lg bg-muted p-0.5" role="radiogroup" aria-label="Who can draw">
        {modes.map(([m, label]) => (
          <button
            key={m}
            type="button"
            role="radio"
            aria-checked={mode === m}
            className={cn(
              'rounded-md px-2.5 py-1 text-xs font-medium',
              mode === m && 'bg-primary text-primary-foreground',
            )}
            onClick={() => boardHost.setMode(m)}
          >
            {label}
          </button>
        ))}
      </div>
      <Button
        size="sm"
        variant="secondary"
        onClick={() => confirm('Clear the whiteboard for everyone?') && op({ t: 'clear' })}
      >
        <Trash2 /> Clear
      </Button>
      <Button size="sm" variant="secondary" onClick={boardHost.close}>
        <X /> Close
      </Button>
    </>
  );
}

function GuestStatus({ me }: { me: string }) {
  const room = useRoomContext();
  const { board, drawers, requests } = useCollabStore((s) => s.host);
  const send = (t: 'draw-request' | 'draw-cancel') =>
    void publish(room, TOPIC.guest, { t }, { to: hostIdentities(room) });

  if (board === 'solo') return <Status>Only the host can draw</Status>;
  if (drawers.includes(me))
    return (
      <Status>
        <Pencil className="size-4" aria-hidden /> You can draw
      </Status>
    );
  if (requests.includes(me)) {
    return (
      <Button size="sm" variant="secondary" onClick={() => send('draw-cancel')}>
        Waiting for approval · Cancel
      </Button>
    );
  }
  return (
    <Button size="sm" onClick={() => send('draw-request')}>
      <Hand /> Request to draw
    </Button>
  );
}

function Status({ children }: { children: React.ReactNode }) {
  return (
    <span className="flex items-center gap-1.5 text-sm text-muted-foreground">{children}</span>
  );
}
