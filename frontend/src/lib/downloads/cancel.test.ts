import { describe, expect, it } from 'vitest';
import { cancelAsks, cancelKeepsFiles } from './cancel';

describe('cancelling a download', () => {
  it('keeps the files of an in-place and of a flat download, and of no other', () => {
    expect(cancelKeepsFiles({ inPlace: true })).toBe(true);
    expect(cancelKeepsFiles({ flat: true })).toBe(true);
    expect(cancelKeepsFiles({})).toBe(false);
    expect(cancelKeepsFiles({ inPlace: false, flat: false })).toBe(false);
  });

  it('asks first only when something was downloaded', () => {
    expect(cancelAsks({ downloaded: 1 })).toBe(true);
    expect(cancelAsks({ downloaded: 0 })).toBe(false);
  });
});
