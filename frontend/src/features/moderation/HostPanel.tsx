import { Check, X } from 'lucide-react';
import { useState } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { errorMessage } from '@/shared/api/client';
import { api } from '@/shared/api/meetings';
import { Alert } from '@/shared/ui/alert';
import { Button } from '@/shared/ui/button';
import { Label } from '@/shared/ui/label';
import { Switch } from '@/shared/ui/switch';
import { useMeetingStore } from '@/stores/meeting';
import { endMeeting } from '@/features/meeting/session';
import { PanelHeader } from '@/features/room/PanelHeader';
import type { WaitingList } from './useWaitingList';

interface Props {
  code: string;
  waiting: WaitingList;
  onClose: () => void;
}

/** Host controls. Rendering is a convenience; every action is authorized by the API. */
export function HostPanel({ code, waiting: { waiting, refresh }, onClose }: Props) {
  const { meeting, set } = useMeetingStore(useShallow((s) => ({ meeting: s.meeting, set: s.set })));
  const [error, setError] = useState<string | null>(null);
  const run = (action: Promise<unknown>) =>
    action.then(refresh, (err: unknown) => setError(errorMessage(err)));

  async function updateSettings(patch: { locked?: boolean; waitingRoom?: boolean }) {
    try {
      const s = await api.host.settings(code, patch);
      if (meeting) set({ meeting: { ...meeting, ...s } });
      setError(null);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function onEnd() {
    if (confirm('End the meeting for everyone?')) setError(await endMeeting(code));
  }

  return (
    <div className="flex h-full flex-col">
      <PanelHeader title="Host controls" onClose={onClose} />
      <div className="flex flex-1 flex-col gap-5 overflow-y-auto p-4">
        {error && <Alert>{error}</Alert>}
        <Setting
          id="lock"
          label="Lock meeting"
          checked={meeting?.locked ?? false}
          onChange={(locked) => void updateSettings({ locked })}
        />
        <Setting
          id="wr"
          label="Waiting room"
          checked={meeting?.waitingRoom ?? false}
          onChange={(waitingRoom) => void updateSettings({ waitingRoom })}
        />
        <section className="flex flex-col gap-2" aria-label="Waiting room">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold">Waiting ({waiting.length})</h3>
            {waiting.length > 1 && (
              <Button size="sm" variant="ghost" onClick={() => void run(api.host.denyAll(code))}>
                Deny all
              </Button>
            )}
          </div>
          {waiting.length === 0 && (
            <p className="text-sm text-muted-foreground">No one is waiting.</p>
          )}
          {waiting.map((p) => (
            <div key={p.id} className="flex items-center gap-2">
              <span className="min-w-0 flex-1 truncate text-sm">{p.displayName}</span>
              <Button
                size="icon"
                variant="secondary"
                aria-label={`Admit ${p.displayName}`}
                onClick={() => void run(api.host.admit(code, p.id))}
              >
                <Check />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                aria-label={`Deny ${p.displayName}`}
                onClick={() => void run(api.host.deny(code, p.id))}
              >
                <X />
              </Button>
            </div>
          ))}
        </section>
        <Button variant="destructive" className="mt-auto" onClick={() => void onEnd()}>
          End meeting for all
        </Button>
      </div>
    </div>
  );
}

function Setting(props: {
  id: string;
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between">
      <Label htmlFor={props.id}>{props.label}</Label>
      <Switch id={props.id} checked={props.checked} onCheckedChange={props.onChange} />
    </div>
  );
}
