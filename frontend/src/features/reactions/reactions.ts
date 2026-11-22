export const REACTION_TOPIC = 'lamma.reaction';

export const EMOJIS = ['👍', '👏', '❤️', '😂', '🎉', '😮', '✋'] as const;
export type Emoji = (typeof EMOJIS)[number];

/** Accepts only emojis from the fixed set, so peers cannot inject text. */
export function parseReaction(v: unknown): Emoji | null {
  if (typeof v !== 'object' || v === null) return null;
  const e = (v as { e?: unknown }).e;
  return EMOJIS.find((x) => x === e) ?? null;
}
