const PRUNE_AT = 1_000;

/**
 * Sliding-window gate: allows at most `max` events per `windowMs` per key.
 * Used to drop floods of peer-to-peer data messages. Idle keys (participants
 * who left) are pruned once the map grows.
 */
export function createRateGate(max: number, windowMs: number, now: () => number = Date.now) {
  const hits = new Map<string, number[]>();
  return (key: string): boolean => {
    const t = now();
    if (hits.size > PRUNE_AT) {
      for (const [k, v] of hits) if (v.every((h) => t - h >= windowMs)) hits.delete(k);
    }
    const recent = (hits.get(key) ?? []).filter((h) => t - h < windowMs);
    const allowed = recent.length < max;
    if (allowed) recent.push(t);
    hits.set(key, recent);
    return allowed;
  };
}
