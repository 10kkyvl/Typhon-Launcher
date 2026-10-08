import { describe, expect, it } from 'vitest';
import type { InstallStatus } from '../services/install';
import { installIndeterminate, installTotalUnknown } from './progress';

describe('install progress', () => {
  it.each<[InstallStatus, number, boolean, boolean, boolean]>([
    ['installing', 0, true, true, true],
    ['installing', -1, true, true, true],
    ['installing', 0, false, false, true],
    ['installing', 371, true, false, false],
    ['verifying', 0, true, false, true],
    ['verifying', 371, true, false, true],
    ['preparing', 0, true, false, false],
    ['pending', 0, true, false, false],
    ['completed', 0, true, false, false],
  ])('%s with %i bytes total, silent=%s: written=%s indeterminate=%s', (status, bytesTotal, silent, written, indeterminate) => {
    expect(installTotalUnknown({ status, bytesTotal, silent })).toBe(written);
    expect(installIndeterminate({ status, bytesTotal, silent })).toBe(indeterminate);
  });
});
