import type { ChatMessage } from '@/shared/api/types';
import { mergeMessages, parseMessage } from './messages';

const msg = (id: string, sentAt: string): ChatMessage => ({
  id,
  participantId: 'p',
  displayName: 'A',
  text: 'hi',
  sentAt,
});

describe('chat messages', () => {
  it('deduplicates and orders by time', () => {
    const merged = mergeMessages(
      [msg('b', '2026-01-01T00:00:02Z')],
      [msg('a', '2026-01-01T00:00:01Z'), msg('b', '2026-01-01T00:00:02Z')],
    );
    expect(merged.map((m) => m.id)).toEqual(['a', 'b']);
  });

  it('parses valid payloads and rejects malformed ones', () => {
    const enc = (v: unknown) => new TextEncoder().encode(JSON.stringify(v));
    expect(parseMessage(enc(msg('x', 't')))).toMatchObject({ id: 'x' });
    expect(parseMessage(enc({ id: 1 }))).toBeNull();
    expect(parseMessage(new TextEncoder().encode('not json'))).toBeNull();
  });
});
