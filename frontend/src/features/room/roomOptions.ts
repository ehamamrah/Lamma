import { type RoomOptions, ScreenSharePresets, VideoPresets } from 'livekit-client';

/**
 * Bandwidth-aware defaults (PLAN.md §15): simulcast with two lower layers,
 * adaptive stream (subscribe at rendered size), dynacast (pause unused
 * layers), 720p capture and 1080p/15fps screen share.
 */
export const roomOptions: RoomOptions = {
  adaptiveStream: true,
  dynacast: true,
  videoCaptureDefaults: { resolution: VideoPresets.h720.resolution },
  publishDefaults: {
    simulcast: true,
    videoSimulcastLayers: [VideoPresets.h180, VideoPresets.h360],
    screenShareEncoding: ScreenSharePresets.h1080fps15.encoding,
    dtx: true,
    red: true,
  },
};
