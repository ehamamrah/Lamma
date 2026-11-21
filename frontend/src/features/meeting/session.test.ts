import { DisconnectReason } from 'livekit-client';
import type { Participant } from '@/shared/api/types';
import { useMeetingStore } from '@/stores/meeting';
import { onDisconnected, stageFor } from './session';

const me = (status: Participant['status']): Participant => ({
  id: '1',
  displayName: 'A',
  role: 'guest',
  status,
});

describe('stageFor', () => {
  it.each([
    [null, 'prejoin'],
    [me('admitted'), 'prejoin'],
    [me('left'), 'prejoin'],
    [me('waiting'), 'waiting'],
    [me('removed'), 'removed'],
    [me('denied'), 'denied'],
  ] as const)('%o → %s', (p, stage) => {
    expect(stageFor(p)).toBe(stage);
  });
});

describe('onDisconnected', () => {
  beforeEach(() => useMeetingStore.getState().reset());

  it.each([
    [DisconnectReason.PARTICIPANT_REMOVED, 'removed'],
    [DisconnectReason.ROOM_DELETED, 'ended'],
    [DisconnectReason.SIGNAL_CLOSE, 'error'],
  ])('maps reason %s to %s while in the room', (reason, stage) => {
    useMeetingStore.getState().set({ stage: 'in_room' });
    onDisconnected(reason);
    expect(useMeetingStore.getState().stage).toBe(stage);
  });

  it('ignores disconnects after the user already left', () => {
    useMeetingStore.getState().set({ stage: 'left' });
    onDisconnected(DisconnectReason.ROOM_DELETED);
    expect(useMeetingStore.getState().stage).toBe('left');
  });
});
