import { useRoomContext } from '@livekit/components-react';
import { type PointerEvent, useCallback, useRef } from 'react';
import { useCollabStore } from '@/stores/collab';
import { publish } from '@/features/room/data';
import { normalise } from './draw';
import { type BoardOp, LIMITS, TOPIC } from '@/features/collab/protocol';

const FLUSH_MS = 50;

interface Pen {
  color: string;
  width: number;
}

/**
 * Pointer handlers for drawing. Points are applied locally at once and sent
 * to peers in small batches (reliable data messages).
 */
export function useBoardDrawing(enabled: boolean, pen: Pen) {
  const room = useRoomContext();
  const current = useRef<{ id: string } & Pen>(null);
  const pending = useRef<number[]>([]);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);

  const flush = useCallback(() => {
    timer.current = undefined;
    const c = current.current;
    while (c && pending.current.length) {
      const points = pending.current.splice(0, LIMITS.segPoints);
      void publish(room, TOPIC.board, { t: 'seg', ...c, points });
    }
    pending.current = [];
  }, [room]);

  const add = (e: PointerEvent<HTMLCanvasElement>) => {
    const c = current.current;
    if (!c) return;
    const points = normalise(e.clientX, e.clientY, e.currentTarget.getBoundingClientRect());
    useCollabStore.getState().apply(room.localParticipant.identity, { t: 'seg', ...c, points });
    pending.current.push(...points);
    timer.current ??= setTimeout(flush, FLUSH_MS);
  };

  const onPointerDown = (e: PointerEvent<HTMLCanvasElement>) => {
    if (!enabled || e.button > 0) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    current.current = { id: strokeId(), ...pen };
    add(e);
  };
  const end = () => {
    clearTimeout(timer.current);
    flush();
    current.current = null;
  };
  return { onPointerDown, onPointerMove: add, onPointerUp: end, onPointerCancel: end };
}

/** Applies an operation locally and sends it to everyone. */
export function useBoardOp() {
  const room = useRoomContext();
  return useCallback(
    (op: BoardOp) => {
      useCollabStore.getState().apply(room.localParticipant.identity, op);
      void publish(room, TOPIC.board, op);
    },
    [room],
  );
}

/** Random stroke id; crypto.randomUUID needs a secure context (not plain-http LAN tests). */
function strokeId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(12));
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}
