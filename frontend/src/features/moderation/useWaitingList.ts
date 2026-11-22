import { useRoomContext } from '@livekit/components-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '@/shared/api/meetings';
import type { Participant } from '@/shared/api/types';
import { onServerData } from '@/features/room/data';

const FALLBACK_REFRESH_MS = 20_000;
const SIGNAL_DEBOUNCE_MS = 500;

export interface WaitingList {
  waiting: Participant[];
  refresh: () => void;
}

/**
 * Host view of the waiting room. Refreshes on the server's "someone is
 * waiting" signal (debounced, host only) with a slow fallback refresh in case
 * a signal was dropped.
 */
export function useWaitingList(code: string): WaitingList {
  const [waiting, setWaiting] = useState<Participant[]>([]);
  const debounce = useRef<ReturnType<typeof setTimeout>>(undefined);
  const refresh = useCallback(() => {
    api.host.waiting(code).then(
      (r) => setWaiting(r.participants),
      () => undefined,
    );
  }, [code]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, FALLBACK_REFRESH_MS);
    return () => {
      clearInterval(id);
      clearTimeout(debounce.current);
    };
  }, [refresh]);

  const room = useRoomContext();
  useEffect(
    () =>
      onServerData(room, 'lamma.waiting', () => {
        clearTimeout(debounce.current);
        debounce.current = setTimeout(refresh, SIGNAL_DEBOUNCE_MS);
      }),
    [room, refresh],
  );
  return { waiting, refresh };
}
