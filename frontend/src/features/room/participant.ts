import type { Participant, Room } from 'livekit-client';

/**
 * Role from server-set metadata. Participants cannot change their own
 * metadata, so this is trustworthy for filtering peer data messages; the API
 * still authorizes every host action.
 */
export function roleOf(p?: Participant): string | undefined {
  try {
    return (JSON.parse(p?.metadata ?? '{}') as { role?: string }).role;
  } catch {
    return undefined;
  }
}

export const isHostParticipant = (p?: Participant) => roleOf(p) === 'host';

/** Identities of the hosts currently in the room (normally one). */
export function hostIdentities(room: Room): string[] {
  return [...room.remoteParticipants.values()].filter(isHostParticipant).map((p) => p.identity);
}
