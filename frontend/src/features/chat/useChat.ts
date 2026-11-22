import { useRoomContext } from '@livekit/components-react';
import { useCallback, useEffect, useState } from 'react';
import { api } from '@/shared/api/meetings';
import { onServerData } from '@/features/room/data';
import type { ChatMessage } from '@/shared/api/types';
import { mergeMessages, parseMessage } from './messages';

/**
 * Chat is sent through the API (validated, rate-limited) and delivered by
 * LiveKit from the server. Peers can publish data (reactions, whiteboard), so
 * only server-sent packets are accepted as chat.
 */
export function useChat(code: string) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);

  useEffect(() => {
    api.chatHistory(code).then(
      (r) => setMessages((cur) => mergeMessages(cur, r.messages)),
      () => undefined,
    );
  }, [code]);

  const room = useRoomContext();
  useEffect(
    () =>
      onServerData(room, 'lamma.chat', (payload) => {
        const m = parseMessage(payload);
        if (m) setMessages((cur) => mergeMessages(cur, [m]));
      }),
    [room],
  );

  const send = useCallback(
    async (text: string) => {
      const m = await api.sendChat(code, text);
      setMessages((cur) => mergeMessages(cur, [m]));
    },
    [code],
  );
  return { messages, send };
}
