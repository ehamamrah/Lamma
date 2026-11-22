import { useMediaDeviceSelect, usePreviewTracks } from '@livekit/components-react';
import { Track, type LocalVideoTrack } from 'livekit-client';
import { Mic, MicOff, Video, VideoOff } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button } from '@/shared/ui/button';
import { useMeetingStore } from '@/stores/meeting';

/** Local camera/microphone preview with toggles and device selection. */
export function DevicePreview() {
  const devices = useMeetingStore((s) => s.devices);
  const set = useMeetingStore((s) => s.set);
  const [deviceError, setDeviceError] = useState<string | null>(null);
  const options = useMemo(
    () => ({
      audio: devices.audio ? { deviceId: devices.audioDeviceId } : false,
      video: devices.video ? { deviceId: devices.videoDeviceId } : false,
    }),
    [devices],
  );
  // The error callback must be stable: usePreviewTracks re-creates the tracks
  // whenever it changes, which would loop (and flicker) on every render.
  const onDeviceError = useCallback((e: Error) => setDeviceError(e.message), []);
  const tracks = usePreviewTracks(options, onDeviceError);
  const video = tracks?.find((t) => t.kind === Track.Kind.Video) as LocalVideoTrack | undefined;

  return (
    <section className="flex flex-col gap-3" aria-label="Device preview">
      <VideoSurface track={video} />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant={devices.audio ? 'secondary' : 'destructive'}
          size="icon"
          aria-label={devices.audio ? 'Turn microphone off' : 'Turn microphone on'}
          onClick={() => set({ devices: { ...devices, audio: !devices.audio } })}
        >
          {devices.audio ? <Mic /> : <MicOff />}
        </Button>
        <Button
          type="button"
          variant={devices.video ? 'secondary' : 'destructive'}
          size="icon"
          aria-label={devices.video ? 'Turn camera off' : 'Turn camera on'}
          onClick={() => set({ devices: { ...devices, video: !devices.video } })}
        >
          {devices.video ? <Video /> : <VideoOff />}
        </Button>
        <DeviceSelect
          kind="audioinput"
          label="Microphone"
          onChange={(id) => set({ devices: { ...devices, audioDeviceId: id } })}
        />
        <DeviceSelect
          kind="videoinput"
          label="Camera"
          onChange={(id) => set({ devices: { ...devices, videoDeviceId: id } })}
        />
      </div>
      {deviceError && (
        <p className="text-sm text-muted-foreground">
          Could not access your devices ({deviceError}). You can still join.
        </p>
      )}
    </section>
  );
}

function VideoSurface({ track }: { track?: LocalVideoTrack }) {
  const ref = useRef<HTMLVideoElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el || !track) return;
    track.attach(el);
    return () => {
      track.detach(el);
    };
  }, [track]);
  return (
    <div className="relative aspect-video overflow-hidden rounded-lg bg-muted">
      {track ? (
        <video ref={ref} className="size-full -scale-x-100 object-cover" muted playsInline />
      ) : (
        <div className="flex size-full items-center justify-center text-muted-foreground">
          Camera is off
        </div>
      )}
    </div>
  );
}

function DeviceSelect(props: {
  kind: MediaDeviceKind;
  label: string;
  onChange: (id: string) => void;
}) {
  const { devices, activeDeviceId, setActiveMediaDevice } = useMediaDeviceSelect({
    kind: props.kind,
  });
  if (devices.length < 2) return null;
  return (
    <select
      aria-label={props.label}
      value={activeDeviceId}
      className="h-10 max-w-48 rounded-lg border border-border bg-background px-2 text-sm"
      onChange={(e) => {
        void setActiveMediaDevice(e.target.value);
        props.onChange(e.target.value);
      }}
    >
      {devices.map((d) => (
        <option key={d.deviceId} value={d.deviceId}>
          {d.label || props.label}
        </option>
      ))}
    </select>
  );
}
