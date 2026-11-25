import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { cachedUsage, cancelScan, deleteScreenshots, deleteSnapshots, scanUsage, type Report } from '../../lib/services/diskusage';
import type { LibraryGame } from '../../lib/services/library';
import type { StorageInfo } from '../../lib/services/system';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { storageInfo } from '../../lib/stores/storage';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import DiskUsage from './DiskUsage.svelte';
import { GB, group, leftover, report, rowOf, groupAsked } from './diskUsageTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  cancelScan: vi.fn(),
  deleteDownloadData: vi.fn(),
  deleteLeftover: vi.fn(),
  deleteScreenshots: vi.fn(),
  deleteSnapshots: vi.fn(),
}));
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  getStorageInfo: vi.fn(async () => null),
}));

async function settle() {
  for (let i = 0; i < 6; i++) {
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((a, b) => ((resolve = a), (reject = b)));
  return { promise, resolve, reject };
}
const A = 'D:\\Games\\.a';
const B = 'D:\\Games\\.b';
const partial = (path: string, patch = {}) => leftover(path, 'install_partial', patch);
const deleteOf = (path: string) => within(rowOf('diskusage.leftoversTitle', path)).getByRole('button', { name: msg('diskusage.leftoverDelete') });
const INSTALLED = [{ id: 'g1', title: 'One', installDir: 'C:\\Games\\One' }] as LibraryGame[];
let flip = false;
const libraryChanges = () => {
  flip = !flip;
  libraryGames.set([{ ...INSTALLED[0], uninstalled: flip }] as LibraryGame[]);
};
let scrolls = 0;
beforeEach(() => {
  flip = false;
  scrolls = 0;
  (Element.prototype as unknown as { scrollIntoView: () => void }).scrollIntoView = () => {
    scrolls += 1;
  };
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(cancelScan).mockResolvedValue();
  libraryGames.set(INSTALLED);
  libraryLoaded.set(true);
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
  libraryLoaded.set(false);
  storageInfo.set(null);
});

// Cases a second review drove: orders of events the screen's other tests do not reach.
describe('DiskUsage: focus when a report replaces another', () => {
  it('leaves focus on the same control, and scrolls nothing, when a rescan only reorders the rows', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 }), partial(B, { sizeBytes: 2 })] }))
      .mockResolvedValueOnce(report({ leftovers: [partial(B, { sizeBytes: 9 }), partial(A, { sizeBytes: 3 })] }));
    renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();
    const before = deleteOf(B);
    before.focus();
    scrolls = 0;
    libraryChanges();
    await settle();
    expect(vi.mocked(scanUsage)).toHaveBeenCalledTimes(2);
    const rows = Array.from(document.querySelectorAll('[data-usage-row]')).map((r) => r.getAttribute('data-usage-row'));
    expect(document.activeElement).toBe(before);
    expect(scrolls).toBe(0);
  });

  it('hands focus on when the focused row\'s whole section leaves', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A)], screenshots: [group('g-h', 'Hades', 4)] }))
      .mockResolvedValueOnce(report({ screenshots: [group('g-h', 'Hades', 4)] }));
    renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();
    deleteOf(A).focus();
    libraryChanges();
    await settle();
    const a = document.activeElement as HTMLElement;
    expect(a).not.toBe(document.body);
  });

  it('hands focus on when the whole report empties', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A)] }))
      .mockResolvedValueOnce(report({}));
    renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();
    deleteOf(A).focus();
    libraryChanges();
    await settle();
    const a = document.activeElement as HTMLElement;
    expect(a).not.toBe(document.body);
  });

  it('keeps the rows and the focus through a failed scan', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A), partial(B)] }))
      .mockRejectedValueOnce(new Error('typhon:diskusage.not_started: x'));
    renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();
    const el = deleteOf(A);
    el.focus();
    libraryChanges();
    await settle();
    expect(screen.getByText(msg('errInstall.diskusageNotStarted'))).toBeTruthy();
    expect(document.activeElement).toBe(el);
  });
});

describe('DiskUsage: a group question is its own kind\'s', () => {
  it('does not follow, or close with, the other kind\'s row of the same game', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ screenshots: [group('g-h', 'Hades', 12)], snapshots: [group('g-h', 'Hades', 4)] }))
      .mockResolvedValueOnce(report({ screenshots: [group('g-h', 'Hades', 30)], snapshots: [group('g-h', 'Hades', 4)] }))
      .mockResolvedValueOnce(report({ snapshots: [group('g-h', 'Hades', 4)] }));
    const { user } = renderDom(DiskUsage);
    await screen.findAllByText('Hades');
    await settle();
    await user.click(within(rowOf('diskusage.snapshotsTitle', 'Hades')).getByRole('button', { name: msg('diskusage.deleteAll') }));
    const dialog = await screen.findByRole('dialog', { name: msg('diskusage.snapshotsConfirmTitle') });
    const text = () => dialog.querySelector(`#${CSS.escape(dialog.getAttribute('aria-describedby') ?? '')}`)?.textContent;
    expect(text()).toBe(groupAsked('snapshots', 'Hades', 4));
    libraryChanges();
    await settle();
    expect(text()).toBe(groupAsked('snapshots', 'Hades', 4));
    libraryChanges();
    await settle();
    expect(screen.queryByRole('dialog')).toBe(dialog);
    expect(text()).toBe(groupAsked('snapshots', 'Hades', 4));
  });
});

describe('DiskUsage: a delete while a scan is in flight', () => {
  it('waits for the scan in flight, which Go restarts after its delete, and asks for no second one', async () => {
    const scans: ReturnType<typeof deferred<Report>>[] = [];
    vi.mocked(scanUsage).mockImplementation(() => {
      const d = deferred<Report>();
      scans.push(d);
      return d.promise;
    });
    vi.mocked(cachedUsage).mockResolvedValue(report({ screenshots: [group('g-h', 'Hades', 12)] }));
    vi.mocked(deleteScreenshots).mockResolvedValue({ deleted: 12, gone: 0, failed: 0, code: '' });
    const { user } = renderDom(DiskUsage);
    await screen.findByText('Hades');
    await settle();
    expect(scans).toHaveLength(1);
    await user.click(within(rowOf('diskusage.screenshotsTitle', 'Hades')).getByRole('button', { name: msg('diskusage.deleteAll') }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('diskusage.deleteAll') }));
    await settle();
    expect(deleteScreenshots).toHaveBeenCalledTimes(1);
    // Go restarted the walk after its delete: the page's scan in flight answers with the disk after it.
    scans[0].resolve(report({}));
    await settle();
    const stillBusy = !!screen.queryByRole('dialog');
    scans[1]?.resolve(report({}));
    await settle();
    expect({ scans: scans.length, busyAfterFirst: stillBusy }).toEqual({ scans: 1, busyAfterFirst: false });
  });
});

describe('DiskUsage: scans that fail or outlive the page', () => {
  it('runs the follow-up after a failed scan; the failure stays said until it answers', async () => {
    const scans: ReturnType<typeof deferred<Report>>[] = [];
    vi.mocked(scanUsage).mockImplementation(() => {
      const d = deferred<Report>();
      scans.push(d);
      return d.promise;
    });
    renderDom(DiskUsage);
    await settle();
    libraryChanges();
    await settle();
    scans[0].reject(new Error('typhon:diskusage.not_started: x'));
    await settle();
    expect(scans).toHaveLength(2);
    expect(screen.getByText(msg('errInstall.diskusageNotStarted'))).toBeTruthy();
    scans[1].resolve(report({ leftovers: [partial(A)] }));
    await settle();
    expect(screen.queryByText(msg('errInstall.diskusageNotStarted'))).toBeNull();
    expect(screen.getByText(A)).toBeTruthy();
  });

  it('drops a left page\'s answer, and scans for the page entered again', async () => {
    const scans: ReturnType<typeof deferred<Report>>[] = [];
    vi.mocked(scanUsage).mockImplementation(() => {
      const d = deferred<Report>();
      scans.push(d);
      return d.promise;
    });
    const first = renderDom(DiskUsage);
    await settle();
    libraryChanges();
    await settle();
    first.unmount();
    renderDom(DiskUsage);
    await settle();
    expect(scans).toHaveLength(2);
    scans[0].resolve(report({ leftovers: [partial(A)] }));
    await settle();
    expect(scans).toHaveLength(2);
    expect(screen.queryByText(A)).toBeNull();
    scans[1].resolve(report({ leftovers: [partial(B)] }));
    await settle();
    expect(screen.getByText(B)).toBeTruthy();
    expect(cancelScan).toHaveBeenCalledTimes(1);
  });
});

describe('DiskUsage: the summary\'s words', () => {
  it('says a known total, not an unknown one, for a report with nothing in it', async () => {
    vi.mocked(scanUsage).mockResolvedValue(report({}));
    renderDom(DiskUsage);
    await screen.findByText(msg('diskusage.emptyTitle'));
    await settle();
    const total = document.querySelector('.summary .legend .total')?.textContent?.replace(/\s+/g, ' ').trim();
    const used = document.querySelector('.summary .figure-value')?.textContent?.trim();
    expect(total).not.toBe(msg('common.meterTotalUnknown'));
  });

  it('names the library disk without doubling the drive letter\'s colon', async () => {
    vi.mocked(scanUsage).mockResolvedValue(report({ leftovers: [partial(A)] }));
    const { getStorageInfo } = await import('../../lib/services/system');
    const disk: StorageInfo = { path: 'D:\\Games', volume: 'D:', filesystem: 'NTFS', totalBytes: 500 * GB, freeBytes: 120 * GB, usedBytes: 380 * GB };
    vi.mocked(getStorageInfo).mockResolvedValue(disk);
    const { settings } = await import('../../lib/stores/settings');
    settings.set({ libraryPath: 'D:\\Games' } as never);
    renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();
    const line = Array.from(document.querySelectorAll('.summary .note')).map((n) => n.textContent?.trim());
    expect(line).toEqual([msg('diskusage.libraryDisk', { volume: 'D:', free: bytesSize(120 * GB), total: bytesSize(500 * GB) })]);
    expect(line[0]).not.toContain('::');
  });
});
