import { Send } from 'lucide-react';
import { type FormEvent, useEffect, useRef, useState } from 'react';
import { errorMessage } from '@/shared/api/client';
import { Alert } from '@/shared/ui/alert';
import { Button } from '@/shared/ui/button';
import { Input } from '@/shared/ui/input';
import { PanelHeader } from '@/features/room/PanelHeader';
import { useChat } from './useChat';

export function ChatPanel({ code, onClose }: { code: string; onClose: () => void }) {
  const { messages, send } = useChat(code);
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => endRef.current?.scrollIntoView({ block: 'end' }), [messages]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    try {
      await send(text);
      setText('');
      setError(null);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  return (
    <div className="flex h-full flex-col">
      <PanelHeader title="Chat" onClose={onClose} />
      <ol className="flex flex-1 flex-col gap-3 overflow-y-auto p-4" aria-live="polite">
        {messages.map((m) => (
          <li key={m.id} className="flex flex-col">
            <span className="text-xs text-muted-foreground">
              {m.displayName} ·{' '}
              {new Date(m.sentAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
            </span>
            <p className="whitespace-pre-wrap break-words text-sm">{m.text}</p>
          </li>
        ))}
        <div ref={endRef} />
      </ol>
      <form className="flex flex-col gap-2 border-t border-border p-3" onSubmit={onSubmit}>
        {error && <Alert>{error}</Alert>}
        <div className="flex gap-2">
          <Input
            aria-label="Message"
            maxLength={1000}
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Send a message"
          />
          <Button type="submit" size="icon" aria-label="Send">
            <Send />
          </Button>
        </div>
      </form>
    </div>
  );
}
