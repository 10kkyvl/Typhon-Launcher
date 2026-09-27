import { describe, expect, it } from 'vitest';
import { nextLibraryFilter } from './filters';

describe('library filter', () => {
  it('selects a filter on the first click and returns to All on the second click', () => {
    let filter = nextLibraryFilter('all', 'installed');
    expect(filter).toBe('installed');

    filter = nextLibraryFilter(filter, 'installed');
    expect(filter).toBe('all');
  });

  it('selects a different filter and keeps All idempotent', () => {
    expect(nextLibraryFilter('installed', 'recent')).toBe('recent');
    expect(nextLibraryFilter('installed', 'all')).toBe('all');
    expect(nextLibraryFilter('all', 'all')).toBe('all');
  });
});
