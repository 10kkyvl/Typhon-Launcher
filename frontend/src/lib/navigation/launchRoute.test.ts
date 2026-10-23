import { describe, expect, it } from 'vitest';
import { launchRoute } from './launchRoute';

describe('launchRoute', () => {
  it.each([
    ['library', 'library'],
    ['downloads', 'downloads'],
    ['settings', null],
    ['', null],
    [undefined, null],
    [42, null],
  ])('maps %j to %j', (value, want) => {
    expect(launchRoute(value)).toBe(want);
  });
});
