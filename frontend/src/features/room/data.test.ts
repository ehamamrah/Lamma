import { RoomEvent, type Room } from 'livekit-client';
import { onServerData } from './data';

function fakeRoom() {
  let listener: ((...args: unknown[]) => void) | undefined;
  const room = {
    on: (_e: RoomEvent, l: (...args: unknown[]) => void) => (listener = l),
    off: () => undefined,
  } as unknown as Room;
  return { room, emit: (...args: unknown[]) => listener?.(...args) };
}

test('only server packets (no sender, no identity) reach the handler', () => {
  const { room, emit } = fakeRoom();
  const handler = vi.fn();
  onServerData(room, 'lamma.chat', handler);
  const payload = new Uint8Array([1]);
  emit(payload, undefined, 0, 'lamma.chat', 0, 'guest-not-yet-known'); // spoof window
  emit(payload, { identity: 'g' }, 0, 'lamma.chat', 0, 'g');
  emit(payload, undefined, 0, 'lamma.other', 0, '');
  expect(handler).not.toHaveBeenCalled();
  emit(payload, undefined, 0, 'lamma.chat', 0, '');
  expect(handler).toHaveBeenCalledTimes(1);
});
