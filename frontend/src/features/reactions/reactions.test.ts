import { parseReaction } from './reactions';

test('accepts only known emojis', () => {
  expect(parseReaction({ e: '🎉' })).toBe('🎉');
  expect(parseReaction({ e: '<img src=x>' })).toBeNull();
  expect(parseReaction('🎉')).toBeNull();
  expect(parseReaction(null)).toBeNull();
});
