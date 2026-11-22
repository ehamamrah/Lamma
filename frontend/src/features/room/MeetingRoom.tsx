import '@livekit/components-styles';
import {
  ConnectionStateToast,
  LayoutContextProvider,
  LiveKitRoom,
  RoomAudioRenderer,
} from '@livekit/components-react';
import { useMemo, useState } from 'react';
import { cn } from '@/shared/lib/utils';
import { useMeetingStore } from '@/stores/meeting';
import { onDisconnected, onRoomError } from '@/features/meeting/session';
import { ChatPanel } from '@/features/chat/ChatPanel';
import { HostPanel } from '@/features/moderation/HostPanel';
import { type WaitingList, useWaitingList } from '@/features/moderation/useWaitingList';
import { ParticipantsPanel } from '@/features/participants/ParticipantsPanel';
import { ControlBar, type Panel } from './ControlBar';
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
  const video = useMemo(
    () => (devices.video ? { deviceId: devices.videoDeviceId } : false),
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
      <ConnectionStateToast />
    </LiveKitRoom>
  );
}

function RoomLayout({ code }: { code: string }) {
  const [panel, setPanel] = useState<Panel>(null);
  const isHost = useMeetingStore((s) => s.me?.role === 'host'); // display only
  return (
    <>
      <div className="flex min-h-0 flex-1">
        <div className="min-w-0 flex-1 p-2">
          <Stage />
        </div>
        <aside
          className={cn(
            'w-full max-w-sm border-l border-border bg-card max-md:fixed max-md:inset-0 max-md:z-20 max-md:max-w-none',
            !panel && 'hidden',
          )}
        >
          {panel === 'chat' && <ChatPanel code={code} onClose={() => setPanel(null)} />}
          {panel === 'people' && <ParticipantsPanel code={code} onClose={() => setPanel(null)} />}
        </aside>
      </div>
      {isHost ? (
        <HostControls code={code} panel={panel} onPanel={setPanel} />
      ) : (
        <ControlBar code={code} panel={panel} onPanel={setPanel} />
      )}
    </>
  );
}

/** Hosts track the waiting room even with the panel closed (badge + panel). */
function HostControls(props: { code: string; panel: Panel; onPanel: (p: Panel) => void }) {
  const waiting: WaitingList = useWaitingList(props.code);
  return (
    <>
      {props.panel === 'host' && (
        <aside className="fixed inset-y-0 right-0 z-30 w-full max-w-sm border-l border-border bg-card shadow-xl">
          <HostPanel code={props.code} waiting={waiting} onClose={() => props.onPanel(null)} />
        </aside>
      )}
      <ControlBar {...props} waitingCount={waiting.waiting.length} />
    </>
  );
}
