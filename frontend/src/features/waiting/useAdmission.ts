import { useEffect } from 'react';
import { api } from '@/shared/api/meetings';
import type { Status } from '@/shared/api/types';

const RETRY_BASE_MS = 5_000;

/**
 * Subscribes to the waiting-room SSE stream and reports the host's decision.
 * EventSource reconnects by itself after normal closes (timeouts, restarts).
 * If the stream is refused (rate limit, 5xx), the status is checked and, while
 * still waiting, the stream is reopened after a jittered backoff.
 */
export function useAdmission(code: string, onDecision: (status: Status | 'gone') => void) {
  useEffect(() => {
    let active = true;
    let es: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const decide = (s: Status | 'gone') => active && onDecision(s);
    const retryLater = () => {
      if (active) timer = setTimeout(open, RETRY_BASE_MS + Math.random() * RETRY_BASE_MS);
    };
    const recheck = () =>
      api
        .me(code)
        .then(
          (me) => (me?.status === 'waiting' ? retryLater() : decide(me?.status ?? 'gone')),
          retryLater,
        );
    function open() {
      es = new EventSource(`/api/v1/meetings/${code}/admission/events`);
      es.addEventListener('decision', (e) => {
        es?.close();
        decide((e as MessageEvent<string>).data as Status);
      });
      es.onerror = () => {
        if (es?.readyState === EventSource.CLOSED) void recheck();
      };
    }
    open();
    return () => {
      active = false;
      clearTimeout(timer);
      es?.close();
    };
  }, [code, onDecision]);
}
