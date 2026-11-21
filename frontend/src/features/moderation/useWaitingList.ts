import { useDataChannel } from '@livekit/components-react';
import { useCallback, useEffect, useState } from 'react';
import { api } from '@/shared/api/meetings';
import type { Participant } from '@/shared/api/types';

const FALLBACK_REFRESH_MS = 20_000;

/**
 * Host view of the waiting room. Refreshes when the server signals a new
 * waiter (LiveKit data, host only) with a slow fallback refresh in case a
 * signal was dropped.
 */
export function useWaitingList(code: string) {
  const [waiting, setWaiting] = useState<Participant[]>([]);
  const refresh = useCallback(() => {
    api.host.waiting(code).then(
      (r) => setWaiting(r.participants),
      () => undefined,
    );
  }, [code]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, FALLBACK_REFRESH_MS);
    return () => clearInterval(id);
  }, [refresh]);

  useDataChannel('lamma.waiting', (msg) => {
    if (!msg.from) refresh();
  });
  return { waiting, refresh };
}
