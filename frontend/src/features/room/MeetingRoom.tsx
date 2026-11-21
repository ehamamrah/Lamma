import '@livekit/components-styles';
import {
  ConnectionStateToast,
  LayoutContextProvider,
  LiveKitRoom,
  RoomAudioRenderer,
} from '@livekit/components-react';
import { useState } from 'react';
import { cn } from '@/shared/lib/utils';
import { useMeetingStore } from '@/stores/meeting';
import { onDisconnected } from '@/features/meeting/session';
import { ChatPanel } from '@/features/chat/ChatPanel';
import { HostPanel } from '@/features/moderation/HostPanel';
import { ParticipantsPanel } from '@/features/participants/ParticipantsPanel';
import { ControlBar, type Panel } from './ControlBar';
import { roomOptions } from './roomOptions';
import { Stage } from './Stage';

export function MeetingRoom({ code }: { code: string }) {
  const grant = useMeetingStore((s) => s.grant);
  const devices = useMeetingStore((s) => s.devices);
  const [panel, setPanel] = useState<Panel>(null);
  if (!grant) return null;

  return (
    <LiveKitRoom
      serverUrl={grant.url}
      token={grant.token}
      connect
      audio={devices.audio ? { deviceId: devices.audioDeviceId } : false}
      video={devices.video ? { deviceId: devices.videoDeviceId } : false}
      options={roomOptions}
      onDisconnected={onDisconnected}
      data-lk-theme="default"
      className="flex h-full flex-col"
    >
      <LayoutContextProvider>
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
            {panel === 'host' && <HostPanel code={code} onClose={() => setPanel(null)} />}
          </aside>
        </div>
        <ControlBar code={code} panel={panel} onPanel={setPanel} />
      </LayoutContextProvider>
      <RoomAudioRenderer />
      <ConnectionStateToast />
    </LiveKitRoom>
  );
}
