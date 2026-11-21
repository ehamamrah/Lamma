import { normalizeCode } from './code';

describe('normalizeCode', () => {
  it.each([
    ['abcd-efgh-jkmn', 'abcd-efgh-jkmn'],
    ['ABCD-EFGH-JKMN', 'abcd-efgh-jkmn'],
    ['  abcdefghjkmn ', 'abcd-efgh-jkmn'],
    ['abcd efgh jkmn', 'abcd-efgh-jkmn'],
    ['https://meet.example.com/m/abcd-efgh-jkmn', 'abcd-efgh-jkmn'],
  ])('normalises %s', (input, expected) => {
    expect(normalizeCode(input)).toBe(expected);
  });

  it.each(['', 'abcd-efgh-jkm', 'abcd-efgh-jkmo', 'abcd-efgh-jkmnp', '../../etc/passwd'])(
    'rejects %s',
    (input) => {
      expect(normalizeCode(input)).toBeNull();
    },
  );
});
