import { describe, expect, it } from 'vitest';
import { nextGenre } from './filters';

describe('catalog genre filter', () => {
  it('selects a genre on the first click and clears it on the second click', () => {
    let genre = nextGenre('', 'Action');
    expect(genre).toBe('Action');

    genre = nextGenre(genre, 'Action');
    expect(genre).toBe('');
  });

  it('selects a different genre and keeps All idempotent', () => {
    expect(nextGenre('Action', 'Adventure')).toBe('Adventure');
    expect(nextGenre('Action', '')).toBe('');
    expect(nextGenre('', '')).toBe('');
  });
});
