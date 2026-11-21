import { type FormEvent, useState } from 'react';
import { Alert } from '@/shared/ui/alert';
import { Button } from '@/shared/ui/button';
import { Card, CardDescription, CardTitle } from '@/shared/ui/card';
import { Field } from '@/shared/ui/field';
import { Input } from '@/shared/ui/input';
import { useMeetingStore } from '@/stores/meeting';
import { enterRoom, joinMeeting } from '@/features/meeting/session';
import { DevicePreview } from './DevicePreview';

/** Device preview plus, for new visitors, name and password entry. */
export function PreJoin({ code }: { code: string }) {
  const { meeting, me, stage, error } = useMeetingStore();
  const [name, setName] = useState(me?.displayName ?? '');
  const [password, setPassword] = useState('');
  const hasSession = me !== null && me.status === 'admitted';
  const busy = stage === 'connecting';

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (hasSession) void enterRoom(code);
    else void joinMeeting(code, name, password || undefined);
  }

  return (
    <main className="mx-auto grid min-h-full max-w-5xl items-center gap-6 p-6 md:grid-cols-[3fr_2fr]">
      <DevicePreview />
      <Card>
        <form className="flex flex-col gap-4" onSubmit={onSubmit}>
          <div>
            <CardTitle>{meeting?.title || 'Ready to join?'}</CardTitle>
            <CardDescription>
              {meeting?.waitingRoom && !hasSession
                ? 'The host will let you in.'
                : 'Check your camera and microphone.'}
            </CardDescription>
          </div>
          <Field id="name" label="Your name">
            <Input
              id="name"
              required
              maxLength={40}
              autoComplete="name"
              value={name}
              disabled={me !== null}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          {meeting?.requiresPassword && !me && (
            <Field id="meeting-password" label="Meeting password">
              <Input
                id="meeting-password"
                type="password"
                required
                autoComplete="off"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
          )}
          {meeting?.locked && !me && <Alert>This meeting is locked by the host.</Alert>}
          {error && <Alert>{error}</Alert>}
          <Button type="submit" size="lg" disabled={busy}>
            {busy ? 'Joining…' : joinLabel(hasSession, meeting?.waitingRoom ?? false)}
          </Button>
        </form>
      </Card>
    </main>
  );
}

function joinLabel(hasSession: boolean, waitingRoom: boolean): string {
  if (hasSession) return 'Join now';
  return waitingRoom ? 'Ask to join' : 'Join';
}
