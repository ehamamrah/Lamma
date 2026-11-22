import { useRoomContext } from '@livekit/components-react';
import { type RemoteParticipant, RoomEvent } from 'livekit-client';
import { useCallback, useEffect, useMemo } from 'react';
import { create } from 'zustand';
import { createRateGate } from '@/shared/lib/rateGate';
import { decode, publish } from '@/features/room/data';
import { type Emoji, REACTION_TOPIC, parseReaction } from './reactions';

export interface FloatingReaction {
  id: number;
  emoji: Emoji;
  name: string;
  left: number; // % from the left edge of the stage
}

const LIFETIME_MS = 3_000;
const MAX_VISIBLE = 30;
let nextId = 0;

/** Only the overlay subscribes, so a reaction never re-renders the stage. */
export const useFloatingReactions = create<{ items: FloatingReaction[] }>()(() => ({ items: [] }));

function show(emoji: Emoji, name: string) {
  const id = nextId++;
  const item = { id, emoji, name, left: 5 + Math.random() * 80 };
  useFloatingReactions.setState((s) => ({ items: [...s.items.slice(-MAX_VISIBLE + 1), item] }));
  setTimeout(
    () => useFloatingReactions.setState((s) => ({ items: s.items.filter((r) => r.id !== id) })),
    LIFETIME_MS,
  );
}

/**
 * Sends and receives emoji reactions (lossy data messages). Floods are
 * dropped per sender and across the whole room.
 */
export function useReactions() {
  const room = useRoomContext();
  const gate = useMemo(() => createRateGate(5, 3_000), []);
  const roomGate = useMemo(() => createRateGate(10, 1_000), []);

  useEffect(() => {
    const onData = (
      payload: Uint8Array,
      from?: RemoteParticipant,
      _k?: unknown,
      topic?: string,
    ) => {
      if (topic !== REACTION_TOPIC || !from) return;
      const emoji = parseReaction(decode(payload));
      if (emoji && gate(from.identity) && roomGate('*')) show(emoji, from.name || 'Guest');
    };
    room.on(RoomEvent.DataReceived, onData);
    return () => {
      room.off(RoomEvent.DataReceived, onData);
      useFloatingReactions.setState({ items: [] });
    };
  }, [room, gate, roomGate]);

  return useCallback(
    (emoji: Emoji) => {
      if (!gate(room.localParticipant.identity)) return;
      show(emoji, 'You');
      void publish(room, REACTION_TOPIC, { e: emoji }, { lossy: true });
    },
    [room, gate],
  );
}
