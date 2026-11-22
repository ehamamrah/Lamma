import { useSyncExternalStore } from 'react';

const supported = () => typeof window !== 'undefined' && typeof window.matchMedia === 'function';

/** Tracks a CSS media query. */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      if (!supported()) return () => undefined;
      const list = window.matchMedia(query);
      list.addEventListener('change', onChange);
      return () => list.removeEventListener('change', onChange);
    },
    () => supported() && window.matchMedia(query).matches,
    () => false,
  );
}

/** Phones and small tablets: below Tailwind's `md` breakpoint. */
export const useIsMobile = () => useMediaQuery('(max-width: 767px)');
