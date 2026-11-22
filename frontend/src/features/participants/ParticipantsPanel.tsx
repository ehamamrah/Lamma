import { useParticipants } from '@livekit/components-react';
import { MicOff, UserMinus, VolumeX } from 'lucide-react';
import { errorMessage } from '@/shared/api/client';
import { api } from '@/shared/api/meetings';
import { useState } from 'react';
import { Alert } from '@/shared/ui/alert';
import { Badge } from '@/shared/ui/badge';
import { Button } from '@/shared/ui/button';
import { useMeetingStore } from '@/stores/meeting';
import { PanelHeader } from '@/features/room/PanelHeader';
import { roleOf } from '@/features/room/participant';

export function ParticipantsPanel({ code, onClose }: { code: string; onClose: () => void }) {
  const participants = useParticipants();
  const isHost = useMeetingStore((s) => s.me?.role === 'host');
  const [error, setErrorText] = useState<string | null>(null);
  const setError = (err: unknown) => setErrorText(errorMessage(err));

  return (
    <div className="flex h-full flex-col">
      <PanelHeader title={`People (${participants.length})`} onClose={onClose} />
      {error && <Alert className="m-3">{error}</Alert>}
      <ul className="flex flex-col divide-y divide-border overflow-y-auto">
        {participants.map((p) => (
          <li key={p.identity} className="flex items-center gap-2 px-4 py-3">
            <span className="min-w-0 flex-1 truncate text-sm">
              {p.name || 'Guest'}{' '}
              {p.isLocal && <span className="text-muted-foreground">(you)</span>}
            </span>
            {roleOf(p) === 'host' && <Badge>Host</Badge>}
            {!p.isMicrophoneEnabled && (
              <MicOff className="size-4 text-muted-foreground" aria-label="Muted" />
            )}
            {isHost && !p.isLocal && (
              <HostActions
                onMute={() => api.host.mute(code, p.identity, 'microphone').catch(setError)}
                onRemove={() => {
                  if (confirm(`Remove ${p.name || 'this participant'} from the meeting?`)) {
                    api.host.remove(code, p.identity).catch(setError);
                  }
                }}
              />
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function HostActions({ onMute, onRemove }: { onMute: () => void; onRemove: () => void }) {
  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        aria-label="Mute microphone"
        title="Mute"
        onClick={onMute}
      >
        <VolumeX />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label="Remove participant"
        title="Remove"
        onClick={onRemove}
      >
        <UserMinus />
      </Button>
    </>
  );
}
