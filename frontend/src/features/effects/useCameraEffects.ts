import { useLocalParticipant, useRoomContext } from '@livekit/components-react';
import type { LocalVideoTrack } from 'livekit-client';
import { useEffect } from 'react';
import { effectsSupported, isActive, useEffectsStore } from './effects';
import { EffectsProcessor } from './processor';

/** A fresh processor when an effect is chosen and the browser can run it. */
export const processorIfActive = () =>
  effectsSupported() && isActive(useEffectsStore.getState()) ? new EffectsProcessor() : undefined;

/**
 * Attaches the effects processor to a camera track while any effect is on,
 * and removes it when none is (so plain video costs nothing extra). The
 * processor reads the current choice every frame, so switching effects is
 * instant. Stopping the track destroys its processor (LiveKit owns that).
 */
export function useCameraEffects(track: LocalVideoTrack | undefined) {
  useEffect(() => {
    if (!track || !effectsSupported()) return;
    let disposed = false;
    let queue = Promise.resolve();
    const sync = () => {
      queue = queue
        .then(async () => {
          if (disposed) return;
          const want = isActive(useEffectsStore.getState());
          const has = track.getProcessor() !== undefined;
          if (want && !has) await track.setProcessor(new EffectsProcessor());
          else if (!want && has) await track.stopProcessor();
        })
        .catch((err: unknown) => {
          if (!disposed) console.warn('camera effects:', err);
        });
    };
    sync();
    const unsubscribe = useEffectsStore.subscribe((s, prev) => {
      if (isActive(s) !== isActive(prev)) sync();
    });
    return () => {
      disposed = true;
      unsubscribe();
    };
  }, [track]);
}

/**
 * In a meeting: process the published camera, and keep LiveKit's capture
 * defaults in step, so a camera that is first turned on mid-meeting is
 * processed before it is published.
 */
export function useRoomCameraEffects() {
  const room = useRoomContext();
  useCameraEffects(useLocalParticipant().cameraTrack?.track as LocalVideoTrack | undefined);
  useEffect(() => {
    // A getter, so each new camera track gets its own processor instead of
    // sharing one that a previous track may still be tearing down.
    const defaults = (room.options.videoCaptureDefaults ??= {});
    Object.defineProperty(defaults, 'processor', {
      get: processorIfActive,
      configurable: true,
      enumerable: true,
    });
  }, [room]);
}
