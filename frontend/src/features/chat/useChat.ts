import { useDataChannel } from '@livekit/components-react';
import { useCallback, useEffect, useState } from 'react';
import { api } from '@/shared/api/meetings';
import type { ChatMessage } from '@/shared/api/types';
import { mergeMessages, parseMessage } from './messages';

/**
 * Chat is sent through the API (validated, rate-limited) and delivered by
 * LiveKit from the server. Clients cannot publish data, so only messages
 * without a participant sender (i.e. from the server) are accepted.
 */
export function useChat(code: string) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);

  useEffect(() => {
    api.chatHistory(code).then(
      (r) => setMessages((cur) => mergeMessages(cur, r.messages)),
      () => undefined,
    );
  }, [code]);

  useDataChannel('lamma.chat', (msg) => {
    if (msg.from) return;
    const m = parseMessage(msg.payload);
    if (m) setMessages((cur) => mergeMessages(cur, [m]));
  });

  const send = useCallback(
    async (text: string) => {
      const m = await api.sendChat(code, text);
      setMessages((cur) => mergeMessages(cur, [m]));
    },
    [code],
  );
  return { messages, send };
}
