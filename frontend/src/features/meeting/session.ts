import { DisconnectReason } from 'livekit-client';
import { ApiError, errorMessage } from '@/shared/api/client';
import { api } from '@/shared/api/meetings';
import type { Participant, Status } from '@/shared/api/types';
import { type MeetingPatch, type Stage, useMeetingStore } from '@/stores/meeting';

// All stage transitions live in this module; components call these functions
// instead of writing `stage` themselves.

const set = (patch: MeetingPatch) => useMeetingStore.getState().set(patch);

/** The meeting currently loaded; guards against responses for a previous code. */
let currentCode: string | null = null;

/** Maps the server-side participant status to the UI stage. */
export function stageFor(me: Participant | null): Stage {
  const byStatus: Partial<Record<Status, Stage>> = {
    waiting: 'waiting',
    removed: 'removed',
    denied: 'denied',
  };
  return (me && byStatus[me.status]) ?? 'prejoin';
}

/** Loads public meeting info and the caller's session (if any). */
export async function loadSession(code: string): Promise<void> {
  currentCode = code;
  useMeetingStore.getState().reset();
  try {
    const [meeting, me] = await Promise.all([api.getMeeting(code), api.me(code)]);
    if (currentCode === code) set({ meeting, me, stage: stageFor(me) });
  } catch (err) {
    if (currentCode !== code) return;
    const notFound = err instanceof ApiError && err.status === 404;
    set({ stage: notFound ? 'not_found' : 'error', error: errorMessage(err) });
  }
}

/** Clears all meeting state when the meeting page unmounts. */
export function closeSession() {
  currentCode = null;
  useMeetingStore.getState().reset();
}

/** Applies a waiting-room decision (or the status found after a stream error). */
export function applyAdmission(code: string, status: Status | 'gone') {
  if (status === 'admitted') void enterRoom(code);
  else if (status === 'denied' || status === 'removed' || status === 'left') set({ stage: status });
  else if (status === 'gone') set({ stage: 'not_found' });
}

/** Ends the meeting for everyone (host). Returns an error message, if any. */
export async function endMeeting(code: string): Promise<string | null> {
  try {
    await api.host.end(code);
    set({ grant: null, stage: 'ended' });
    return null;
  } catch (err) {
    return errorMessage(err);
  }
}

/** Handles a failure to connect to LiveKit. */
export function onRoomError() {
  set({ grant: null, stage: 'error', error: 'Could not connect to the media server.' });
}

/** Joins (or resumes), then enters the room or the waiting room. */
export async function joinMeeting(code: string, displayName: string, password?: string) {
  set({ error: null, stage: 'connecting' });
  try {
    const { participant } = await api.join(code, displayName, password);
    set({ me: participant });
    if (participant.status === 'waiting') set({ stage: 'waiting' });
    else await enterRoom(code);
  } catch (err) {
    set({ stage: 'prejoin', error: errorMessage(err) });
  }
}

/** Claims a seat and obtains a short-lived LiveKit token. */
export async function enterRoom(code: string) {
  set({ error: null, stage: 'connecting' });
  try {
    const grant = await api.token(code);
    set({ grant, stage: 'in_room' });
  } catch (err) {
    set({ stage: 'prejoin', error: errorMessage(err) });
  }
}

/** Leaves voluntarily; the session is kept so the user can rejoin. */
export async function leaveMeeting(code: string) {
  set({ grant: null, stage: 'left' });
  await api.leave(code).catch(() => undefined);
}

/** Translates a LiveKit disconnect into the next stage. */
export function onDisconnected(reason?: DisconnectReason) {
  const { stage } = useMeetingStore.getState();
  if (stage !== 'in_room') return; // already handled (leave / end)
  const next: Partial<Record<DisconnectReason, Stage>> = {
    [DisconnectReason.PARTICIPANT_REMOVED]: 'removed',
    [DisconnectReason.ROOM_DELETED]: 'ended',
    [DisconnectReason.CLIENT_INITIATED]: 'left',
  };
  set({ grant: null, stage: (reason !== undefined && next[reason]) || 'error' });
}
