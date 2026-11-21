// Meeting codes are 12 Crockford-base32 symbols shown as xxxx-xxxx-xxxx.
// The API accepts only the canonical lowercase dashed form (PLAN.md §8), so
// the SPA normalises anything a user types or pastes before calling it.

const ALPHABET = /^[0-9a-hjkmnp-tv-z]{12}$/;

/** Returns the canonical code, or null if input cannot be a meeting code. */
export function normalizeCode(input: string): string | null {
  const fromUrl = input.trim().split('/m/').pop() ?? '';
  const compact = fromUrl.toLowerCase().replace(/[^0-9a-z]/g, '');
  if (!ALPHABET.test(compact)) return null;
  return `${compact.slice(0, 4)}-${compact.slice(4, 8)}-${compact.slice(8, 12)}`;
}
