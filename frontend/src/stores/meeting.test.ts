import { useMeetingStore } from './meeting';

describe('meeting store', () => {
  beforeEach(() => useMeetingStore.getState().reset());

  it('starts loading with media on', () => {
    const s = useMeetingStore.getState();
    expect(s.stage).toBe('loading');
    expect(s.devices).toEqual({ audio: true, video: true });
  });

  it('applies partial updates and resets', () => {
    useMeetingStore.getState().set({ stage: 'waiting', error: 'x' });
    expect(useMeetingStore.getState()).toMatchObject({ stage: 'waiting', error: 'x' });
    useMeetingStore.getState().reset();
    expect(useMeetingStore.getState().stage).toBe('loading');
  });
});
