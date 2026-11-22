import { type Room, RoomEvent } from 'livekit-client';

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/** Parses JSON; returns null for anything malformed. */
export function parseJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

export const decode = (payload: Uint8Array): unknown => parseJson(decoder.decode(payload));

/** Publishes a JSON message on a topic; failures are ignored (best effort). */
export function publish(
  room: Room,
  topic: string,
  msg: unknown,
  opts: { to?: string[]; lossy?: boolean } = {},
): Promise<void> {
  return room.localParticipant
    .publishData(encoder.encode(JSON.stringify(msg)), {
      topic,
      reliable: !opts.lossy,
      destinationIdentities: opts.to,
    })
    .catch(() => undefined);
}

/**
 * Subscribes to data the API sent through LiveKit on `topic`. Server packets
 * carry no participant identity, which peers cannot forge; checking for a
 * missing sender object instead is unsafe, because it is also missing while
 * the sender is not yet (or no longer) known locally.
 */
export function onServerData(room: Room, topic: string, handler: (payload: Uint8Array) => void) {
  const listener = (
    payload: Uint8Array,
    from?: unknown,
    _kind?: unknown,
    t?: string,
    _enc?: unknown,
    identity?: string,
  ) => {
    if (t === topic && !from && !identity) handler(payload);
  };
  room.on(RoomEvent.DataReceived, listener);
  return () => {
    room.off(RoomEvent.DataReceived, listener);
  };
}
