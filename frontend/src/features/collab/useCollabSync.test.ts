import type { RemoteParticipant, Room } from 'livekit-client';
import { boardHost, useCollabStore } from '@/stores/collab';
import { TOPIC } from './protocol';
import { type Gates, route } from './useCollabSync';

const person = (identity: string, role: 'host' | 'guest') =>
  ({ identity, metadata: JSON.stringify({ role }) }) as unknown as RemoteParticipant;
const host = person('h', 'host');
const guest = person('g', 'guest');
const room = () =>
  ({
    localParticipant: {
      publishData: vi.fn(() => Promise.resolve()),
      sendText: vi.fn(() => Promise.resolve()),
    },
  }) as unknown as Room;
const allow: Gates = {
  board: () => true,
  guest: () => true,
  hello: () => true,
  helloAll: () => true,
};
const seg = (id: string) => ({ t: 'seg', id, color: '#111827', width: 5, points: [0.1, 0.1] });
const state = (patch: object) => ({
  t: 'state',
  recording: false,
  board: 'off',
  drawers: [],
  requests: [],
  ...patch,
});
const store = () => useCollabStore.getState();

beforeEach(() => store().reset());

describe('as a guest', () => {
  test('host state is accepted only from the host', () => {
    route(room(), false, guest, TOPIC.host, state({ recording: true }), allow);
    expect(store().host.recording).toBe(false);
    route(room(), false, host, TOPIC.host, state({ recording: true }), allow);
    expect(store().host.recording).toBe(true);
  });

  test('strokes need the host’s permission; clear is host-only', () => {
    route(room(), false, host, TOPIC.host, state({ board: 'collab' }), allow);
    route(room(), false, guest, TOPIC.board, seg('g1'), allow);
    expect(store().strokes).toHaveLength(0);

    route(room(), false, host, TOPIC.host, state({ board: 'collab', drawers: ['g'] }), allow);
    route(room(), false, guest, TOPIC.board, seg('g1'), allow);
    route(room(), false, host, TOPIC.board, seg('h1'), allow);
    expect(store().strokes).toHaveLength(2);

    route(room(), false, guest, TOPIC.board, { t: 'clear' }, allow);
    expect(store().strokes).toHaveLength(2);
    route(room(), false, host, TOPIC.board, { t: 'clear' }, allow);
    expect(store().strokes).toHaveLength(0);
  });
});

describe('as the host', () => {
  test('draw requests are recorded, and rate-gated', () => {
    boardHost.open('collab');
    route(
      room(),
      true,
      guest,
      TOPIC.guest,
      { t: 'draw-request' },
      { ...allow, guest: () => false },
    );
    expect(store().host.requests).toEqual([]);
    route(room(), true, guest, TOPIC.guest, { t: 'draw-request' }, allow);
    expect(store().host.requests).toEqual(['g']);
  });

  test('a guest cannot overwrite the host’s state', () => {
    boardHost.open('solo');
    route(room(), true, guest, TOPIC.host, state({ board: 'collab', drawers: ['g'] }), allow);
    expect(store().host).toMatchObject({ board: 'solo', drawers: [] });
  });

  test('hello is answered with the current state', () => {
    const r = room();
    route(r, true, guest, TOPIC.guest, { t: 'hello' }, allow);
    expect(r.localParticipant.publishData).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ topic: TOPIC.host, destinationIdentities: ['g'] }),
    );
  });
});

test('stroke floods from one drawer are dropped', () => {
  route(room(), false, host, TOPIC.host, state({ board: 'solo' }), allow);
  route(room(), false, host, TOPIC.board, seg('h1'), { ...allow, board: () => false });
  expect(store().strokes).toHaveLength(0);
});
