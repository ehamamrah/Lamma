import { useEffect } from 'react';
import { api } from '@/shared/api/meetings';
import type { Status } from '@/shared/api/types';

/**
 * Subscribes to the waiting-room SSE stream and reports the host's decision.
 * EventSource reconnects automatically after timeouts and server restarts;
 * if the stream is refused (e.g. status changed), the current status is
 * fetched once instead.
 */
export function useAdmission(code: string, onDecision: (status: Status | 'gone') => void) {
  useEffect(() => {
    const es = new EventSource(`/api/v1/meetings/${code}/admission/events`);
    es.addEventListener('decision', (e) => {
      es.close();
      onDecision((e as MessageEvent<string>).data as Status);
    });
    es.onerror = () => {
      if (es.readyState !== EventSource.CLOSED) return;
      void api.me(code).then((me) => onDecision(me?.status ?? 'gone'));
    };
    return () => es.close();
  }, [code, onDecision]);
}
