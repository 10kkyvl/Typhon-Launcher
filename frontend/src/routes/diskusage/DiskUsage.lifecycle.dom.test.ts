import { cleanup, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { cachedUsage, scanUsage, type Report } from '../../lib/services/diskusage';
import type { MoveJob } from '../../lib/services/relocate';
import { moves } from '../../lib/stores/relocate';
import { renderDom } from '../../lib/testing/dom';
import DiskUsage from './DiskUsage.svelte';

// What the page reads, and when: the page module's first read on mount (the cached report, shown
// before the scan answers, then the scan) and the rescan when a move ends, which stops with the page.

const P = vi.hoisted(() => ({ log: [] as string[] }));

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
}));

const report = (title: string) =>
  ({
    games: [{ id: 'g1', title, installDir: 'C:\\Games\\x', sizeBytes: 1, sizeUnknown: false, lastPlayed: null }],
    downloads: [],
    leftovers: [],
    screenshots: [],
    snapshots: [],
    warnings: [],
    totals: { gamesBytes: 1, downloadsBytes: 0, leftoversBytes: 0, screenshotsBytes: 0, snapshotsBytes: 0, totalBytes: 1, reclaimableBytes: 0, unknownSizes: false },
    scannedAt: '2026-11-12T00:00:00Z',
  }) as unknown as Report;

const move = (stage: MoveJob['stage']) => ({ id: 'm1', gameId: 'g1', title: 'Hades', stage }) as MoveJob;

async function settle() {
  for (let i = 0; i < 5; i++) {
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

beforeEach(() => {
  P.log = [];
  vi.mocked(cachedUsage).mockImplementation(async () => {
    P.log.push('cached');
    return report('From the cache');
  });
  vi.mocked(scanUsage).mockImplementation(() => {
    P.log.push('scan');
    return new Promise(() => {});
  });
  moves.set([]);
});

afterEach(() => {
  cleanup();
  moves.set([]);
  vi.clearAllMocks();
});

describe('the disk usage page', () => {
  it('reads the cached report, shows it, then scans once', async () => {
    renderDom(DiskUsage);
    expect(await screen.findByText('From the cache')).toBeTruthy();
    await settle();
    expect(P.log).toEqual(['cached', 'scan']);
  });

  it('scans again when a move ends, and not after it is left', async () => {
    const view = renderDom(DiskUsage);
    await settle();
    expect(P.log).toEqual(['cached', 'scan']);

    moves.set([move('copy')]);
    await settle();
    expect(P.log).toEqual(['cached', 'scan']);
    moves.set([move('done')]);
    await settle();
    expect(P.log).toEqual(['cached', 'scan', 'scan']);

    moves.set([move('copy')]);
    await settle();
    view.unmount();
    moves.set([move('done')]);
    await settle();
    expect(P.log).toEqual(['cached', 'scan', 'scan']);
  });

  it('keeps the scan error in view while the cached report stays', async () => {
    vi.mocked(scanUsage).mockImplementation(async () => {
      P.log.push('scan');
      throw new Error('typhon:diskusage.not_scanned');
    });
    renderDom(DiskUsage);
    expect(await screen.findByText(msg('errInstall.diskusageNotScanned'))).toBeTruthy();
    expect(screen.getByText('From the cache')).toBeTruthy();
  });
});
