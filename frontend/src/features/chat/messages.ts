import type { ChatMessage } from '@/shared/api/types';

const MAX_MESSAGES = 200;

/** Merges messages by id (API echo + data channel), oldest first, capped. */
export function mergeMessages(current: ChatMessage[], incoming: ChatMessage[]): ChatMessage[] {
  const byId = new Map(current.map((m) => [m.id, m]));
  for (const m of incoming) byId.set(m.id, m);
  return [...byId.values()].sort((a, b) => a.sentAt.localeCompare(b.sentAt)).slice(-MAX_MESSAGES);
}

/** Parses a data-channel payload; returns null for anything malformed. */
export function parseMessage(payload: Uint8Array): ChatMessage | null {
  try {
    const m = JSON.parse(new TextDecoder().decode(payload)) as Partial<ChatMessage>;
    const ok = [m.id, m.participantId, m.displayName, m.text, m.sentAt].every(
      (v) => typeof v === 'string',
    );
    return ok ? (m as ChatMessage) : null;
  } catch {
    return null;
  }
}
