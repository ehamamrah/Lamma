import { type FormEvent, useState } from 'react';
import { useNavigate } from 'react-router';
import { errorMessage } from '@/shared/api/client';
import { api } from '@/shared/api/meetings';
import { Alert } from '@/shared/ui/alert';
import { Button } from '@/shared/ui/button';
import { Card, CardDescription, CardTitle } from '@/shared/ui/card';
import { Field } from '@/shared/ui/field';
import { Input } from '@/shared/ui/input';
import { Label } from '@/shared/ui/label';
import { Switch } from '@/shared/ui/switch';

export function CreateMeetingForm() {
  const navigate = useNavigate();
  const [form, setForm] = useState({
    displayName: '',
    title: '',
    password: '',
    waitingRoom: false,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const { meeting } = await api.createMeeting({
        displayName: form.displayName,
        title: form.title || undefined,
        password: form.password || undefined,
        waitingRoom: form.waitingRoom,
      });
      navigate(`/m/${meeting.code}`);
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <Card>
      <form className="flex flex-col gap-4" onSubmit={onSubmit}>
        <div>
          <CardTitle>Start a meeting</CardTitle>
          <CardDescription>You'll be the host.</CardDescription>
        </div>
        <Field id="host-name" label="Your name">
          <Input
            id="host-name"
            required
            maxLength={40}
            autoComplete="name"
            value={form.displayName}
            onChange={(e) => setForm({ ...form, displayName: e.target.value })}
          />
        </Field>
        <Field id="title" label="Meeting title (optional)">
          <Input
            id="title"
            maxLength={120}
            value={form.title}
            onChange={(e) => setForm({ ...form, title: e.target.value })}
          />
        </Field>
        <Field id="password" label="Password (optional)" hint="At least 8 characters.">
          <Input
            id="password"
            type="password"
            minLength={8}
            maxLength={64}
            autoComplete="new-password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
          />
        </Field>
        <div className="flex items-center justify-between">
          <Label htmlFor="waiting-room">Waiting room</Label>
          <Switch
            id="waiting-room"
            checked={form.waitingRoom}
            onCheckedChange={(waitingRoom) => setForm({ ...form, waitingRoom })}
          />
        </div>
        {error && <Alert>{error}</Alert>}
        <Button type="submit" size="lg" disabled={busy}>
          {busy ? 'Creating…' : 'New meeting'}
        </Button>
      </form>
    </Card>
  );
}
