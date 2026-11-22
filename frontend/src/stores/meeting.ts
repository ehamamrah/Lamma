import { create } from 'zustand';
import type { MeetingInfo, Participant, TokenGrant } from '@/shared/api/types';

/** Where the visitor is in the meeting lifecycle. */
export type Stage =
  | 'loading'
  | 'not_found'
  | 'prejoin'
  | 'waiting'
  | 'connecting'
  | 'in_room'
  | 'left'
  | 'removed'
  | 'denied'
  | 'ended'
  | 'error';

export interface DevicePrefs {
  audio: boolean;
  video: boolean;
  audioDeviceId?: string;
  videoDeviceId?: string;
}

export interface MeetingData {
  stage: Stage;
  meeting: MeetingInfo | null;
  me: Participant | null;
  grant: TokenGrant | null;
  devices: DevicePrefs;
  error: string | null;
}

/** A partial update of the meeting state. */
export type MeetingPatch = Partial<MeetingData>;

interface MeetingState {
  stage: Stage;
  meeting: MeetingInfo | null;
  me: Participant | null;
  grant: TokenGrant | null;
  devices: DevicePrefs;
  error: string | null;
  set: (patch: MeetingPatch) => void;
  reset: () => void;
}

const initial = {
  stage: 'loading' as Stage,
  meeting: null,
  me: null,
  grant: null,
  devices: { audio: true, video: true },
  error: null,
};

/**
 * Client state for the current meeting. Media state lives in LiveKit, not here;
 * authorization lives on the server — `me.role` is for display only.
 */
export const useMeetingStore = create<MeetingState>()((set) => ({
  ...initial,
  set: (patch) => set(patch),
  reset: () => set(initial),
}));
