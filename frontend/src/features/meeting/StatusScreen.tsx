import { Loader2 } from 'lucide-react';
import { Link } from 'react-router';
import { Button } from '@/shared/ui/button';
import { Card, CardDescription, CardTitle } from '@/shared/ui/card';
import { type Stage, useMeetingStore } from '@/stores/meeting';
import { loadSession } from './session';

const copy: Partial<Record<Stage, { title: string; body: string; rejoin?: boolean }>> = {
  left: {
    title: 'You left the meeting',
    body: 'You can rejoin while the meeting is running.',
    rejoin: true,
  },
  removed: { title: 'You were removed', body: 'The host removed you from this meeting.' },
  denied: { title: 'Request declined', body: 'The host did not let you in.' },
  ended: { title: 'Meeting ended', body: 'The host ended this meeting for everyone.' },
  error: { title: 'Connection lost', body: 'Something went wrong.', rejoin: true },
};

export function StatusScreen({ code, stage }: { code: string; stage: Stage }) {
  const error = useMeetingStore((s) => s.error);
  if (stage === 'loading') {
    return (
      <main className="flex h-full items-center justify-center" aria-busy>
        <Loader2 className="size-8 animate-spin text-muted-foreground" aria-label="Loading" />
      </main>
    );
  }
  const c = copy[stage] ?? copy.error!;
  return (
    <main className="flex h-full items-center justify-center p-6">
      <Card className="flex max-w-md flex-col gap-4 text-center">
        <CardTitle>{c.title}</CardTitle>
        <CardDescription>{stage === 'error' && error ? error : c.body}</CardDescription>
        <div className="flex justify-center gap-2">
          {c.rejoin && <Button onClick={() => void loadSession(code)}>Rejoin</Button>}
          <Button variant="outline" asChild>
            <Link to="/">Home</Link>
          </Button>
        </div>
      </Card>
    </main>
  );
}
