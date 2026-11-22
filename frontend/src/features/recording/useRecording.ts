import { useRoomContext } from '@livekit/components-react';
import { type Room, RoomEvent, Track, type TrackPublication } from 'livekit-client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useCollabStore } from '@/stores/collab';
import { type Recording, canRecord, downloadRecording, startRecording } from './recorder';

/**
 * Host-only local recording. The recording flag is part of the host state,
 * so every participant sees the indicator while it runs.
 */
export function useRecording(code: string) {
  const room = useRoomContext();
  const recording = useCollabStore((s) => s.host.recording);
  const active = useRef<Recording | null>(null);
  const [error, setError] = useState<string | null>(null);

  const start = useCallback(async () => {
    if (active.current) return;
    try {
      const rec = await startRecording();
      active.current = rec;
      const unfollow = followAudio(room, rec);
      useCollabStore.getState().setHost({ recording: true });
      setError(null);
      void rec.done.then((blob) => {
        unfollow();
        active.current = null;
        useCollabStore.getState().setHost({ recording: false });
        if (blob.size > 0) downloadRecording(blob, code);
      });
    } catch (err) {
      // Cancelling the screen picker is not an error worth showing.
      if (!(err instanceof DOMException && err.name === 'NotAllowedError')) {
        setError('Recording could not start in this browser.');
      }
    }
  }, [room, code]);

  const stop = useCallback(() => active.current?.stop(), []);
  useEffect(() => () => active.current?.stop(), []);
  return { supported: canRecord(), recording, start, stop, error };
}

/** Mixes in every current and future audio track (local mic and remote). */
function followAudio(room: Room, rec: Recording): () => void {
  const add = (pub: TrackPublication) => {
    const t = pub.track?.mediaStreamTrack;
    if (pub.kind === Track.Kind.Audio && t) rec.addAudio(t);
  };
  for (const p of [room.localParticipant, ...room.remoteParticipants.values()]) {
    p.trackPublications.forEach(add);
  }
  const onRemote = (_t: unknown, pub: TrackPublication) => add(pub);
  room.on(RoomEvent.TrackSubscribed, onRemote);
  room.on(RoomEvent.LocalTrackPublished, add);
  return () => {
    room.off(RoomEvent.TrackSubscribed, onRemote);
    room.off(RoomEvent.LocalTrackPublished, add);
  };
}
