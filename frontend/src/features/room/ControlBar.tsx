import { TrackToggle } from '@livekit/components-react';
import { Track } from 'livekit-client';
import { Link2, LogOut, MessageSquare, Shield, Users } from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/shared/ui/button';
import { useMeetingStore } from '@/stores/meeting';
import { leaveMeeting } from '@/features/meeting/session';

export type Panel = 'chat' | 'people' | 'host' | null;

interface Props {
  code: string;
  panel: Panel;
  onPanel: (p: Panel) => void;
}

export function ControlBar({ code, panel, onPanel }: Props) {
  const isHost = useMeetingStore((s) => s.me?.role === 'host'); // display only
  const toggle = (p: Exclude<Panel, null>) => onPanel(panel === p ? null : p);

  return (
    <nav
      className="flex flex-wrap items-center justify-center gap-2 border-t border-border p-3"
      aria-label="Meeting controls"
    >
      <TrackToggle source={Track.Source.Microphone} />
      <TrackToggle source={Track.Source.Camera} />
      <TrackToggle source={Track.Source.ScreenShare} captureOptions={{ audio: true }} />
      <PanelButton
        label="Chat"
        active={panel === 'chat'}
        onClick={() => toggle('chat')}
        icon={<MessageSquare />}
      />
      <PanelButton
        label="People"
        active={panel === 'people'}
        onClick={() => toggle('people')}
        icon={<Users />}
      />
      {isHost && (
        <PanelButton
          label="Host controls"
          active={panel === 'host'}
          onClick={() => toggle('host')}
          icon={<Shield />}
        />
      )}
      <CopyLinkButton code={code} />
      <Button variant="destructive" onClick={() => void leaveMeeting(code)}>
        <LogOut /> Leave
      </Button>
    </nav>
  );
}

function PanelButton(props: {
  label: string;
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
}) {
  return (
    <Button
      variant={props.active ? 'default' : 'secondary'}
      size="icon"
      aria-label={props.label}
      aria-pressed={props.active}
      title={props.label}
      onClick={props.onClick}
    >
      {props.icon}
    </Button>
  );
}

function CopyLinkButton({ code }: { code: string }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    await navigator.clipboard.writeText(`${window.location.origin}/m/${code}`);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }
  return (
    <Button variant="secondary" onClick={() => void copy()} aria-live="polite">
      <Link2 /> {copied ? 'Copied' : 'Copy link'}
    </Button>
  );
}
