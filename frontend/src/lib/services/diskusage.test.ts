import { beforeEach, describe, expect, it, vi } from 'vitest';

const S = vi.hoisted(() => ({
  CancelScan: vi.fn(),
  DeleteScreenshots: vi.fn(),
  DeleteSnapshots: vi.fn(),
}));

vi.mock('./backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/diskusage', () => ({ Service: S }));

import { cancelScan, deleteScreenshots, deleteSnapshots } from './diskusage';

beforeEach(() => {
  vi.clearAllMocks();
});

// The wrappers are the screen's one way to the disk usage service: what Go answers reaches the page as
// it was sent, and nothing else is called.
describe('the disk usage service wrappers', () => {
  it('stop the running scan through CancelScan', async () => {
    S.CancelScan.mockResolvedValue(undefined);
    await cancelScan();
    expect(S.CancelScan).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['screenshots', deleteScreenshots, S.DeleteScreenshots],
    ['snapshots', deleteSnapshots, S.DeleteSnapshots],
  ] as const)('answer what deleting the %s of a game did, by the game’s id', async (_name, wrapper, binding) => {
    binding.mockResolvedValue({ deleted: 3, gone: 1, failed: 2, code: 'diskusage.delete_partial' });
    expect(await wrapper('g1')).toEqual({ deleted: 3, gone: 1, failed: 2, code: 'diskusage.delete_partial' });
    expect(binding.mock.calls).toEqual([['g1']]);
  });

  it('read a code they do not know as no code, keeping the counts', async () => {
    S.DeleteScreenshots.mockResolvedValue({ deleted: 2, gone: 0, failed: 0, code: 'diskusage.other' });
    expect(await deleteScreenshots('g1')).toEqual({ deleted: 2, gone: 0, failed: 0, code: '' });
  });
});
