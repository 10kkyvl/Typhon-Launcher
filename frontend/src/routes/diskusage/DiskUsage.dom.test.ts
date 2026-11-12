import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { cachedUsage, scanUsage, type Report } from '../../lib/services/diskusage';
import { renderDom } from '../../lib/testing/dom';
import DiskUsage from './DiskUsage.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
}));

const report = {
  games: [{ id: 'g1', title: 'Hades', installDir: 'C:\\Games\\Hades', sizeBytes: 1024, sizeUnknown: false, lastPlayed: null }],
  downloads: [],
  leftovers: [],
  screenshots: [],
  snapshots: [],
  warnings: [],
  totals: { gamesBytes: 1024, downloadsBytes: 0, leftoversBytes: 0, screenshotsBytes: 0, snapshotsBytes: 0, totalBytes: 1024, reclaimableBytes: 0, unknownSizes: false },
  scannedAt: '2026-11-12T00:00:00Z',
} as unknown as Report;

beforeEach(() => {
  vi.mocked(cachedUsage).mockReset();
  vi.mocked(scanUsage).mockReset();
});

afterEach(() => {
  document.body.replaceChildren();
});

describe('DiskUsage: a section with nothing in it', () => {
  it('says so with the base empty state, as a line under its heading', async () => {
    vi.mocked(cachedUsage).mockResolvedValue(report);
    vi.mocked(scanUsage).mockResolvedValue(report);
    renderDom(DiskUsage);

    for (const key of ['diskusage.downloadsEmpty', 'diskusage.leftoversEmpty', 'diskusage.screenshotsEmpty', 'diskusage.snapshotsEmpty'] as const) {
      const line = await screen.findByText(msg(key));
      expect(line.closest('.empty.inline'), key).not.toBeNull();
    }
  });
});
