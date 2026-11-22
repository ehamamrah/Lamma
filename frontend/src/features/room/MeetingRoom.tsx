import '@livekit/components-styles';
import {
  ConnectionStateToast,
  LayoutContextProvider,
  LiveKitRoom,
  RoomAudioRenderer,
  StartAudio,
} from '@livekit/components-react';
import { Circle, PenLine, Shield } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Alert } from '@/shared/ui/alert';
import { cn } from '@/shared/lib/utils';
import { boardHost, useCollabStore } from '@/stores/collab';
import { useMeetingStore } from '@/stores/meeting';
import { onDisconnected, onRoomError } from '@/features/meeting/session';
import { ChatPanel } from '@/features/chat/ChatPanel';
import { HostPanel } from '@/features/moderation/HostPanel';
import { useWaitingList } from '@/features/moderation/useWaitingList';
import { ParticipantsPanel } from '@/features/participants/ParticipantsPanel';
import { ReactionOverlay } from '@/features/reactions/ReactionOverlay';
import type { Emoji } from '@/features/reactions/reactions';
import { useReactions } from '@/features/reactions/useReactions';
import { RecordingBadge } from '@/features/recording/RecordingBadge';
import { useRecording } from '@/features/recording/useRecording';
import { useCollabSync } from '@/features/collab/useCollabSync';
import { EffectsPicker } from '@/features/effects/EffectsPicker';
import { processorIfActive, useRoomCameraEffects } from '@/features/effects/useCameraEffects';
import type { Action } from './actions';
import { ControlBar, type Panel } from './ControlBar';
import { PanelHeader } from './PanelHeader';
import { roomOptions } from './roomOptions';
import { Stage } from './Stage';

export function MeetingRoom({ code }: { code: string }) {
  const grant = useMeetingStore((s) => s.grant);
  const devices = useMeetingStore((s) => s.devices);
  // Stable references: LiveKitRoom re-runs its setup when these change.
  const audio = useMemo(
    () => (devices.audio ? { deviceId: devices.audioDeviceId } : false),
    [devices.audio, devices.audioDeviceId],
  );
  // With an effect already chosen, the processor is attached before the
  // camera is published, so the first frame others see is already processed.
  // (Creating one is cheap: no work starts until LiveKit initialises it.)
  const video = useMemo(
    () =>
      devices.video ? { deviceId: devices.videoDeviceId, processor: processorIfActive() } : false,
    [devices.video, devices.videoDeviceId],
  );
  if (!grant) return null;

  return (
    <LiveKitRoom
      serverUrl={grant.url}
      token={grant.token}
      connect
      audio={audio}
      video={video}
      options={roomOptions}
      onDisconnected={onDisconnected}
      onError={onRoomError}
      data-lk-theme="default"
      className="flex h-full flex-col"
    >
      <LayoutContextProvider>
        <RoomLayout code={code} />
      </LayoutContextProvider>
      <RoomAudioRenderer />
      {/* Mobile browsers block audio until the user taps. */}
      <StartAudio
        label="Tap to turn on sound"
        className="fixed left-1/2 top-4 z-50 -translate-x-1/2"
      />
      <ConnectionStateToast />
    </LiveKitRoom>
  );
}

function RoomLayout({ code }: { code: string }) {
  const [panel, setPanel] = useState<Panel>(null);
  const isHost = useMeetingStore((s) => s.me?.role === 'host'); // display only
  const react = useReactions();
  useCollabSync(isHost);
  useRoomCameraEffects();
  const bar = { code, panel, onPanel: setPanel, onReact: react };

  return (
    <>
      <div className="flex min-h-0 flex-1">
        <div className="relative min-w-0 flex-1 p-1 md:p-2">
          <Stage />
          <ReactionOverlay />
          <RecordingBadge />
        </div>
        <aside
          className={cn(
            'w-full max-w-sm border-l border-border bg-card max-md:fixed max-md:inset-0 max-md:z-20 max-md:max-w-none',
            // The host panel has its own overlay container (HostControls).
            (!panel || panel === 'host') && 'hidden',
          )}
        >
          {panel === 'chat' && <ChatPanel code={code} onClose={() => setPanel(null)} />}
          {panel === 'people' && <ParticipantsPanel code={code} onClose={() => setPanel(null)} />}
          {panel === 'effects' && (
            <div className="flex h-full flex-col">
              <PanelHeader title="Effects" onClose={() => setPanel(null)} />
              <EffectsPicker className="overflow-y-auto p-4" />
            </div>
          )}
        </aside>
      </div>
      {isHost ? <HostControls {...bar} /> : <ControlBar {...bar} />}
    </>
  );
}

interface BarProps {
  code: string;
  panel: Panel;
  onPanel: (p: Panel) => void;
  onReact: (e: Emoji) => void;
}

/** Hosts also get the whiteboard, recording and the host panel (with badges). */
function HostControls(props: BarProps) {
  const waiting = useWaitingList(props.code);
  const rec = useRecording(props.code);
  const board = useCollabStore((s) => s.host.board);
  const requests = useCollabStore((s) => s.host.requests.length);
  const extra: Action[] = [
    {
      key: 'board',
      label: board === 'off' ? 'Open whiteboard' : 'Close whiteboard',
      icon: <PenLine />,
      active: board !== 'off',
      badge: requests,
      onClick: () => (board === 'off' ? boardHost.open('solo') : boardHost.close()),
    },
    {
      key: 'host',
      label: 'Host controls',
      icon: <Shield />,
      active: props.panel === 'host',
      badge: waiting.waiting.length,
      onClick: () => props.onPanel(props.panel === 'host' ? null : 'host'),
    },
  ];
  if (rec.supported) {
    extra.splice(1, 0, {
      key: 'record',
      label: rec.recording ? 'Stop recording' : 'Record',
      icon: <Circle className={cn(rec.recording && 'fill-current')} />,
      danger: rec.recording,
      onClick: () => (rec.recording ? rec.stop() : void rec.start()),
    });
  }
  return (
    <>
      {props.panel === 'host' && (
        <aside className="fixed inset-y-0 right-0 z-30 w-full max-w-sm border-l border-border bg-card shadow-xl">
          <HostPanel code={props.code} waiting={waiting} onClose={() => props.onPanel(null)} />
        </aside>
      )}
      {rec.error && (
        <Alert className="fixed left-1/2 top-4 z-40 -translate-x-1/2">{rec.error}</Alert>
      )}
      <ControlBar {...props} extra={extra} />
    </>
  );
}
