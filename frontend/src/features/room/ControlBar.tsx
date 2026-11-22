import { TrackToggle, useTrackToggle } from '@livekit/components-react';
import { Track } from 'livekit-client';
import { Ellipsis, Link2, LogOut, MessageSquare, MonitorUp, Users } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Button } from '@/shared/ui/button';
import { useIsMobile } from '@/shared/lib/useMediaQuery';
import { leaveMeeting } from '@/features/meeting/session';
import { ReactionPicker } from '@/features/reactions/ReactionPicker';
import type { Emoji } from '@/features/reactions/reactions';
import { type Action, ActionButton, MoreSheet } from './actions';

export type Panel = 'chat' | 'people' | 'host' | null;

interface Props {
  code: string;
  panel: Panel;
  onPanel: (p: Panel) => void;
  onReact: (e: Emoji) => void;
  /** Host-only actions (whiteboard, recording, host controls). */
  extra?: Action[];
}

const canShareScreen = () => typeof navigator.mediaDevices?.getDisplayMedia === 'function';

/**
 * Mic, camera, reactions, chat and leave are always visible. Everything else
 * is inline on desktop and in a "More" sheet on phones.
 */
export function ControlBar({ code, panel, onPanel, onReact, extra = [] }: Props) {
  const isMobile = useIsMobile();
  const [more, setMore] = useState(false);
  const secondary = [...useSecondaryActions(code, panel, onPanel), ...extra];
  const pending = secondary.reduce((n, a) => n + (a.badge ?? 0), 0);

  return (
    <nav
      className="relative flex items-center justify-center gap-2 border-t border-border p-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] md:flex-wrap md:p-3"
      aria-label="Meeting controls"
    >
      <TrackToggle source={Track.Source.Microphone} />
      <TrackToggle source={Track.Source.Camera} />
      <ReactionPicker onReact={onReact} />
      <ActionButton
        action={{
          key: 'chat',
          label: 'Chat',
          icon: <MessageSquare />,
          active: panel === 'chat',
          onClick: () => onPanel(panel === 'chat' ? null : 'chat'),
        }}
      />
      {isMobile ? (
        <ActionButton
          action={{
            key: 'more',
            label: 'More options',
            icon: <Ellipsis />,
            badge: pending,
            onClick: () => setMore(true),
          }}
        />
      ) : (
        secondary.map((a) => <ActionButton key={a.key} action={a} />)
      )}
      <Button
        variant="destructive"
        size={isMobile ? 'icon' : 'default'}
        aria-label="Leave"
        onClick={() => void leaveMeeting(code)}
      >
        <LogOut /> {!isMobile && 'Leave'}
      </Button>
      {more && isMobile && <MoreSheet actions={secondary} onClose={() => setMore(false)} />}
    </nav>
  );
}

function useSecondaryActions(code: string, panel: Panel, onPanel: (p: Panel) => void): Action[] {
  const screen = useTrackToggle({
    source: Track.Source.ScreenShare,
    captureOptions: { audio: true },
  });
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const id = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(id);
  }, [copied]);
  const copy = () =>
    navigator.clipboard.writeText(`${window.location.origin}/m/${code}`).then(
      () => setCopied(true),
      () => undefined, // clipboard blocked: nothing to confirm
    );
  const actions: Action[] = [
    {
      key: 'people',
      label: 'People',
      icon: <Users />,
      active: panel === 'people',
      onClick: () => onPanel(panel === 'people' ? null : 'people'),
    },
    {
      key: 'copy',
      label: copied ? 'Copied' : 'Copy link',
      icon: <Link2 />,
      wide: true,
      onClick: () => void copy(),
    },
  ];
  if (canShareScreen()) {
    const label = screen.enabled ? 'Stop sharing' : 'Share screen';
    actions.unshift({
      key: 'screen',
      label,
      icon: <MonitorUp />,
      active: screen.enabled,
      onClick: () => void screen.toggle(),
    });
  }
  return actions;
}
