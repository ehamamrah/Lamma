import { createRateGate } from './rateGate';

test('allows max events per window per key', () => {
  let t = 0;
  const gate = createRateGate(2, 1000, () => t);
  expect([gate('a'), gate('a'), gate('a'), gate('b')]).toEqual([true, true, false, true]);
  t = 1001;
  expect(gate('a')).toBe(true);
});
