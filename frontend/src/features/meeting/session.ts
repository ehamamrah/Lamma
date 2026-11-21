import { DisconnectReason } from 'livekit-client';
import { ApiError, errorMessage } from '@/shared/api/client';
import { api } from '@/shared/api/meetings';
import type { Participant, Status } from '@/shared/api/types';
import { type Stage, useMeetingStore } from '@/stores/meeting';

const set = (patch: Parameters<ReturnType<typeof useMeetingStore.getState>['set']>[0]) =>
  useMeetingStore.getState().set(patch);

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
  useMeetingStore.getState().reset();
  try {
    const [meeting, me] = await Promise.all([api.getMeeting(code), api.me(code)]);
    set({ meeting, me, stage: stageFor(me) });
  } catch (err) {
    const notFound = err instanceof ApiError && err.status === 404;
    set({ stage: notFound ? 'not_found' : 'error', error: errorMessage(err) });
  }
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
