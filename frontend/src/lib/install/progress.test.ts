import { describe, expect, it } from 'vitest';
import type { InstallStatus } from '../services/install';
import { installIndeterminate, installTotalUnknown } from './progress';

describe('install progress', () => {
  it.each<[InstallStatus, number, boolean, boolean]>([
    ['installing', 0, true, true],
    ['installing', -1, true, true],
    ['installing', 371, false, false],
    ['verifying', 0, false, true],
    ['verifying', 371, false, true],
    ['preparing', 0, false, false],
    ['pending', 0, false, false],
    ['completed', 0, false, false],
  ])('%s with %i bytes total: unknown=%s indeterminate=%s', (status, bytesTotal, unknown, indeterminate) => {
    expect(installTotalUnknown({ status, bytesTotal })).toBe(unknown);
    expect(installIndeterminate({ status, bytesTotal })).toBe(indeterminate);
  });
});
