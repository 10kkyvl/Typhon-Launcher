import { cleanup, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { cachedUsage, scanUsage } from '../../lib/services/diskusage';
import type { StorageInfo } from '../../lib/services/system';
import { refreshStorage, storageInfo } from '../../lib/stores/storage';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import DiskUsage from './DiskUsage.svelte';
import { GB, download, game, group, leftover, report } from './diskUsageTesting';

// The summary on the shared disk meter: one segment per kind of the launcher's files from Go's totals
// (each byte once), the leftovers split into what can be deleted and what cannot, the library disk's
// size and free space when measured (else said as not measured), sizes not measured said as "at
// least", and under it Go's totals in words. A segment picked leads to its section. Services and the
// storage store's refresh are mocked: nothing is measured.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  cancelScan: vi.fn(),
}));
vi.mock('../../lib/stores/storage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/storage')>()),
  refreshStorage: vi.fn(),
}));

const shown = report({
  games: [game('g1', 'Hades', { sizeBytes: 40 * GB, sizeUnknown: true })],
  downloads: [download('d1', 'Spare', { sizeBytes: 10 * GB })],
  leftovers: [
    leftover('D:\\Games\\.part', 'install_partial', { sizeBytes: 3 * GB }),
    leftover('E:\\Games\\Hades', 'relocate_copy_target', { sizeBytes: 5 * GB, deletable: false, reasonCode: 'diskusage.reason.only_copy' }),
  ],
  screenshots: [group('g1', 'Hades', 4, { sizeBytes: 2 * GB })],
  snapshots: [group('g1', 'Hades', 2, { sizeBytes: GB })],
  totals: { gamesBytes: 40 * GB, downloadsBytes: 10 * GB, leftoversBytes: 8 * GB, screenshotsBytes: 2 * GB, snapshotsBytes: GB, totalBytes: 61 * GB, reclaimableBytes: 13 * GB, unknownSizes: true },
});

const disk: StorageInfo = { path: 'C:\\Games', volume: 'C:', filesystem: 'NTFS', totalBytes: 100 * GB, freeBytes: 20 * GB, usedBytes: 80 * GB };

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(scanUsage).mockResolvedValue(shown);
  vi.mocked(refreshStorage).mockResolvedValue();
  storageInfo.set(disk);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  storageInfo.set(null);
});

async function meter() {
  const view = renderDom(DiskUsage);
  await screen.findByText('Spare');
  const bar = document.querySelector<HTMLElement>('.meter .bar');
  if (!bar) throw new Error('no disk meter');
  return { ...view, bar, legend: document.querySelector('.meter .legend') as HTMLElement };
}

const entries = (legend: HTMLElement) =>
  [...legend.querySelectorAll(':scope > li:not(.total)')].map((li) => li.textContent?.replace(/\s+/g, ' ').trim());

const SEGMENTS = [
  `${msg('diskusage.gamesTitle')} ${msg('common.meterAtLeast', { size: bytesSize(40 * GB) })}`,
  `${msg('diskusage.screenshotsTitle')} ${bytesSize(2 * GB)}`,
  `${msg('diskusage.snapshotsTitle')} ${bytesSize(GB)}`,
  `${msg('diskusage.segmentDownloads')} ${bytesSize(10 * GB)}`,
  `${msg('diskusage.segmentReclaim')} ${bytesSize(3 * GB)}`,
  `${msg('diskusage.segmentKept')} ${bytesSize(5 * GB)}`,
];

const diskLine = (info: StorageInfo) => msg('diskusage.libraryDisk', { volume: info.volume, free: bytesSize(info.freeBytes), total: bytesSize(info.totalBytes) });

describe('DiskUsage: the summary meter', () => {
  // The launcher's files lie on every drive; the library disk is one of them. The meter draws the
  // files against their own total, and the disk is said apart from it.
  it('draws each kind of the launcher’s files once, against their own total, and says the library disk on a line of its own', async () => {
    const { bar, legend } = await meter();
    expect(bar.getAttribute('aria-label')).toContain(msg('diskusage.meterLauncher'));
    expect(entries(legend).slice(0, SEGMENTS.length)).toEqual(SEGMENTS);
    expect(entries(legend).join(' ')).not.toContain(msg('common.meterOther'));
    expect(entries(legend).join(' ')).not.toContain(msg('common.meterRest'));
    expect(entries(legend).join(' ')).not.toContain(bytesSize(20 * GB));
    expect(legend.querySelector('.total')?.textContent?.trim()).toBe(msg('common.meterTotal', { total: bytesSize(61 * GB) }));
    expect(screen.getByText(diskLine(disk))).toBeTruthy();
    expect(screen.queryByText(msg('diskusage.freeUnknown'))).toBeNull();
  });

  it('does not draw the files of every drive against a library disk smaller than they are', async () => {
    const small: StorageInfo = { ...disk, totalBytes: 50 * GB, freeBytes: 5 * GB, usedBytes: 45 * GB };
    storageInfo.set(small);
    const { bar } = await meter();
    expect(bar.getAttribute('aria-label')).not.toContain(msg('common.meterOverflow'));
    expect(screen.getByText(diskLine(small))).toBeTruthy();
  });

  it('says Go’s totals under the meter in words, at least where a size is not measured', async () => {
    await meter();
    const summary = document.querySelector('.summary') as HTMLElement;
    expect(within(summary).getByText(msg('diskusage.atLeast', { size: bytesSize(61 * GB) }))).toBeTruthy();
    expect(within(summary).getByText(msg('diskusage.atLeast', { size: bytesSize(13 * GB) }))).toBeTruthy();
    expect(screen.getByText(msg('diskusage.unknownNote', { count: 1 }))).toBeTruthy();
  });

  it('measures the library disk again with the report', async () => {
    await meter();
    expect(refreshStorage).toHaveBeenCalled();
  });

  it('without a measured disk, draws the launcher’s files the same and says the free space is not measured', async () => {
    storageInfo.set(null);
    const { bar, legend } = await meter();
    expect(bar.getAttribute('aria-label')).toContain(msg('diskusage.meterLauncher'));
    expect(entries(legend).slice(0, SEGMENTS.length)).toEqual(SEGMENTS);
    expect(entries(legend)).not.toContain(expect.stringContaining(msg('common.meterOther')));
    expect(screen.getByText(msg('diskusage.freeUnknown'))).toBeTruthy();
    expect(document.querySelector('.summary')?.textContent).not.toContain(msg('diskusage.libraryDisk', { volume: '', free: '', total: '' }).split(':')[0]);
  });

  it('leads a picked segment to its section’s heading', async () => {
    const { user, bar } = await meter();
    await user.click(within(bar).getByRole('button', { name: new RegExp(`^${msg('diskusage.segmentKept')}`) }));
    const heading = document.activeElement as HTMLElement;
    expect(heading.tagName).toBe('H2');
    expect(heading.textContent?.trim().startsWith(msg('diskusage.leftoversTitle'))).toBe(true);

    await user.click(within(bar).getByRole('button', { name: new RegExp(`^${msg('diskusage.segmentDownloads')}`) }));
    expect((document.activeElement as HTMLElement).textContent?.trim().startsWith(msg('diskusage.downloadsTitle'))).toBe(true);
  });
});
