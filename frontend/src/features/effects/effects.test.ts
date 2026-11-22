import { isActive, loadSettings } from './effects';

describe('loadSettings', () => {
  it('restores saved choices', () => {
    expect(loadSettings('{"background":"ocean","mask":"cat"}')).toEqual({
      background: 'ocean',
      mask: 'cat',
    });
  });

  it('ignores unknown, missing or broken values', () => {
    const none = { background: 'none', mask: 'none' };
    expect(loadSettings(null)).toEqual(none);
    expect(loadSettings('not json')).toEqual(none);
    expect(loadSettings('{"background":"evil","mask":42}')).toEqual(none);
  });

  it('never restores a custom image background (the image is not saved)', () => {
    expect(loadSettings('{"background":"custom"}').background).toBe('none');
  });
});

it('isActive', () => {
  expect(isActive({ background: 'none', mask: 'none' })).toBe(false);
  expect(isActive({ background: 'blur', mask: 'none' })).toBe(true);
  expect(isActive({ background: 'none', mask: 'crown' })).toBe(true);
});
