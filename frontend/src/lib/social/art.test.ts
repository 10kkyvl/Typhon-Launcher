import { describe, expect, it } from 'vitest';
import { wideArt } from './art';

describe('wideArt', () => {
  it('prefers the wide shot', () => {
    expect(wideArt({ coverUrl: 'cover.jpg', heroUrl: 'hero.jpg' })).toBe('hero.jpg');
  });

  it('falls back to the cover when the backend sent no hero', () => {
    expect(wideArt({ coverUrl: 'cover.jpg' })).toBe('cover.jpg');
    expect(wideArt({ coverUrl: 'cover.jpg', heroUrl: '' })).toBe('cover.jpg');
  });

  it('survives a missing card', () => {
    expect(wideArt(null)).toBe('');
    expect(wideArt({ coverUrl: '' })).toBe('');
  });
});
