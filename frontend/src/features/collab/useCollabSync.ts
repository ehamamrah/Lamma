import { useRoomContext } from '@livekit/components-react';
import { ConnectionState, type RemoteParticipant, type Room, RoomEvent } from 'livekit-client';
import { useEffect } from 'react';
import { createRateGate } from '@/shared/lib/rateGate';
import { boardHost, useCollabStore } from '@/stores/collab';
import { decode, parseJson, publish } from '@/features/room/data';
import { hostIdentities, isHostParticipant } from '@/features/room/participant';
import {
  type GuestMsg,
  type Stroke,
  TOPIC,
  canDraw,
  parseBoardOp,
  parseGuestMsg,
  parseHostState,
  parseSnapshot,
} from './protocol';

const store = () => useCollabStore.getState();

/**
 * Keeps recording and whiteboard state in sync over LiveKit data messages.
 * The host is the authority: it announces state and answers late joiners
 * with a snapshot. Everyone applies stroke operations only from senders the
 * host's state allows.
 */
export function useCollabSync(amHost: boolean) {
  const room = useRoomContext();
  useEffect(() => {
    const gates = createGates();
    const onData = (
      payload: Uint8Array,
      from?: RemoteParticipant,
      _k?: unknown,
      topic?: string,
    ) => {
      if (from && topic) route(room, amHost, from, topic, decode(payload), gates);
    };
    room.on(RoomEvent.DataReceived, onData);
    return () => {
      room.off(RoomEvent.DataReceived, onData);
    };
  }, [room, amHost]);
  useEffect(() => (amHost ? broadcastHostState(room) : greetHosts(room)), [room, amHost]);
  useEffect(() => (amHost ? undefined : receiveSnapshots(room)), [room, amHost]);
  useEffect(() => () => store().reset(), []);
}

type Gate = (key: string) => boolean;
export interface Gates {
  board: Gate; // stroke messages per drawer
  guest: Gate; // guest → host messages per guest
  hello: Gate; // snapshot requests per guest
  helloAll: Gate; // snapshot requests from everyone
}

/** Receive-side flood limits (per sender unless noted). */
export const createGates = (): Gates => ({
  board: createRateGate(40, 1_000),
  guest: createRateGate(10, 10_000),
  hello: createRateGate(3, 60_000),
  helloAll: createRateGate(5, 1_000),
});

/** Applies one incoming data message. Exported for tests. */
export function route(
  room: Room,
  amHost: boolean,
  from: RemoteParticipant,
  topic: string,
  msg: unknown,
  gates: Gates,
) {
  const id = from.identity;
  if (topic === TOPIC.board && gates.board(id)) applyRemoteOp(from, msg);
  else if (topic === TOPIC.host && !amHost && isHostParticipant(from)) {
    const state = parseHostState(msg);
    if (state) store().setHost(state);
  } else if (topic === TOPIC.guest && amHost && gates.guest(id)) {
    const guestMsg = parseGuestMsg(msg);
    if (guestMsg?.t !== 'hello' || (gates.hello(id) && gates.helloAll('*'))) {
      onGuestMessage(room, id, guestMsg);
    }
  }
}

function applyRemoteOp(from: RemoteParticipant, msg: unknown) {
  const op = parseBoardOp(msg);
  const senderIsHost = isHostParticipant(from);
  if (!op || !canDraw(store().host, from.identity, senderIsHost)) return;
  if (op.t === 'clear' && !senderIsHost) return;
  store().apply(from.identity, op);
}

function onGuestMessage(room: Room, identity: string, msg: GuestMsg | null) {
  if (msg?.t === 'hello') sendStateTo(room, identity);
  else if (msg?.t === 'draw-request') boardHost.request(identity);
  else if (msg?.t === 'draw-cancel') boardHost.deny(identity);
}

let snapshotCache: { strokes: Stroke[]; text: string } | null = null;

/** The serialised board, rebuilt only when the strokes change. */
function snapshotText(strokes: Stroke[]): string {
  if (snapshotCache?.strokes !== strokes) {
    snapshotCache = { strokes, text: JSON.stringify({ strokes }) };
  }
  return snapshotCache.text;
}

/** Host → one late joiner: current state plus every stroke. */
function sendStateTo(room: Room, identity: string) {
  const { host, strokes } = store();
  void publish(room, TOPIC.host, { t: 'state', ...host }, { to: [identity] });
  if (strokes.length === 0) return;
  room.localParticipant
    .sendText(snapshotText(strokes), {
      topic: TOPIC.snapshot,
      destinationIdentities: [identity],
    })
    .catch(() => undefined);
}

/** Host: announce every state change to everyone. */
function broadcastHostState(room: Room) {
  return useCollabStore.subscribe((s, prev) => {
    if (s.host !== prev.host) void publish(room, TOPIC.host, { t: 'state', ...s.host });
  });
}

/** Guest: ask the host for state on connect and whenever a host (re)joins. */
function greetHosts(room: Room) {
  const hello = () => {
    const to = hostIdentities(room);
    if (to.length > 0) void publish(room, TOPIC.guest, { t: 'hello' }, { to });
  };
  const onJoin = (p: RemoteParticipant) => isHostParticipant(p) && hello();
  // The host's browser holds the board. When it leaves, drop our copy, so a
  // host that reloads and its guests cannot end up with different boards.
  const onLeave = (p: RemoteParticipant) => isHostParticipant(p) && store().reset();
  room.on(RoomEvent.Connected, hello);
  room.on(RoomEvent.ParticipantConnected, onJoin);
  room.on(RoomEvent.ParticipantDisconnected, onLeave);
  if (room.state === ConnectionState.Connected) hello();
  return () => {
    room.off(RoomEvent.Connected, hello);
    room.off(RoomEvent.ParticipantConnected, onJoin);
    room.off(RoomEvent.ParticipantDisconnected, onLeave);
  };
}

/** Guest: accept whiteboard snapshots, from the host only. */
function receiveSnapshots(room: Room) {
  room.registerTextStreamHandler(TOPIC.snapshot, (reader, { identity }) => {
    if (!isHostParticipant(room.remoteParticipants.get(identity))) return;
    reader.readAll().then(
      (text) => {
        const strokes = parseSnapshot(parseJson(text));
        if (strokes) store().mergeSnapshot(strokes);
      },
      () => undefined,
    );
  });
  return () => room.unregisterTextStreamHandler(TOPIC.snapshot);
}
