import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../../lib/i18n';
import {
  cachedUsage,
  deleteDownloadData,
  deleteLeftover,
  deleteScreenshots,
  deleteSnapshots,
  scanUsage,
  type GroupUsage,
  type Report,
} from '../../lib/services/diskusage';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { toast } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import DiskUsage from './DiskUsage.svelte';
import { GB, download, group, leftover, report, rowOf } from './diskUsageTesting';

// A group delete (screenshots, save copies) sends only the game's id; Go deletes the set of its latest
// scan. So the open question follows the group's row in every new report: it says what Go would delete
// at the press of Confirm or Retry, closes when the group has gone, and stays untouched by a report
// that changes nothing in its row. The single-row deletes send their own path or id, which Go checks
// again, and keep what was pressed. The services are mocked whole: nothing is deleted.

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
vi.mock('../../lib/stores/toasts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(),
}));

async function settle() {
  for (let i = 0; i < 6; i++) {
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

const INSTALLED = [{ id: 'g1', title: 'One', installDir: 'C:\\Games\\One' }] as LibraryGame[];
// A game leaves (or comes back to) the installed games: the page measures again on its own.
let installedFlip = false;
function libraryChanges() {
  installedFlip = !installedFlip;
  libraryGames.set([{ ...INSTALLED[0], uninstalled: installedFlip }] as LibraryGame[]);
}

interface Kind {
  name: 'screenshots' | 'snapshots';
  section: MessageKey;
  title: MessageKey;
  confirm: MessageKey;
  count: MessageKey;
  text: MessageKey;
  service: typeof deleteScreenshots | typeof deleteSnapshots;
  done: MessageKey;
}

const KINDS: Kind[] = [
  {
    name: 'screenshots',
    section: 'diskusage.screenshotsTitle',
    title: 'diskusage.screenshotsConfirmTitle',
    confirm: 'diskusage.deleteAll',
    count: 'diskusage.screenshotsCount',
    text: 'diskusage.screenshotsConfirmSized',
    service: deleteScreenshots,
    done: 'diskusage.screenshotsDeleted',
  },
  {
    name: 'snapshots',
    section: 'diskusage.snapshotsTitle',
    title: 'diskusage.snapshotsConfirmTitle',
    confirm: 'diskusage.snapshotsConfirmAction',
    count: 'diskusage.snapshotsCount',
    text: 'diskusage.snapshotsConfirmSized',
    service: deleteSnapshots,
    done: 'diskusage.snapshotsDeleted',
  },
];

const withGroups = (kind: Kind, groups: GroupUsage[]): Report => report({ [kind.name]: groups });
const asked = (kind: Kind, title: string, count: number, size: string) => msg(kind.text, { title, count: msg(kind.count, { count }), size });
const textOf = (dialog: HTMLElement) => dialog.querySelector(`#${CSS.escape(dialog.getAttribute('aria-describedby') ?? '')}`)?.textContent;

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  libraryGames.set(INSTALLED);
  libraryLoaded.set(true);
  installedFlip = false;
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
  libraryLoaded.set(false);
});

async function openFor(kind: Kind, row: string) {
  const view = renderDom(DiskUsage);
  await screen.findByText(row);
  await settle();
  await view.user.click(within(rowOf(kind.section, row)).getByRole('button', { name: msg('diskusage.deleteAll') }));
  const dialog = await screen.findByRole('dialog', { name: msg(kind.title) });
  return { ...view, dialog };
}

describe.each(KINDS.map((k) => [k.name, k] as const))('DiskUsage: the question of a group of %s', (_name, kind) => {
  it('asks with the count and size of the row', async () => {
    vi.mocked(scanUsage).mockResolvedValue(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 2 * GB })]));
    const { dialog } = await openFor(kind, 'Hades');
    expect(textOf(dialog)).toBe(asked(kind, 'Hades', 12, bytesSize(2 * GB)));
  });

  it('follows a rescan that changed the group, and Confirm then deletes what it says', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 2 * GB })]))
      .mockResolvedValue(withGroups(kind, [group('g-h', 'Hades', 40, { sizeBytes: 7 * GB })]));
    vi.mocked(kind.service).mockResolvedValue({ deleted: 40, gone: 0, failed: 0, code: '' });
    const { user, dialog } = await openFor(kind, 'Hades');

    libraryChanges();
    await settle();
    expect(scanUsage).toHaveBeenCalledTimes(2);
    expect(screen.getByRole('dialog', { name: msg(kind.title) })).toBe(dialog);
    expect(textOf(dialog)).toBe(asked(kind, 'Hades', 40, bytesSize(7 * GB)));

    await user.click(within(dialog).getByRole('button', { name: msg(kind.confirm) }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(kind.service).mock.calls).toEqual([['g-h']]);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg(kind.done), 'success']]);
  });

  it('follows a change of the size alone, and of a size that is no longer fully measured', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 2 * GB })]))
      .mockResolvedValueOnce(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 3 * GB })]))
      .mockResolvedValueOnce(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 3 * GB, sizeUnknown: true })]));
    const { dialog } = await openFor(kind, 'Hades');

    libraryChanges();
    await settle();
    expect(textOf(dialog)).toBe(asked(kind, 'Hades', 12, bytesSize(3 * GB)));

    libraryChanges();
    await settle();
    expect(textOf(dialog)).toBe(asked(kind, 'Hades', 12, msg('diskusage.atLeast', { size: bytesSize(3 * GB) })));
  });

  it('is left untouched by a rescan that changes nothing in its row', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 2 * GB })]))
      .mockResolvedValue(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 2 * GB }), group('g-o', 'Other', 3)]));
    const { dialog } = await openFor(kind, 'Hades');
    const focused = document.activeElement;
    const changes: MutationRecord[] = [];
    const observer = new MutationObserver((records) => changes.push(...records));
    observer.observe(dialog, { subtree: true, childList: true, characterData: true, attributes: true });

    libraryChanges();
    await settle();
    observer.disconnect();
    expect(screen.getByText('Other')).toBeTruthy();
    expect(changes).toEqual([]);
    expect(document.activeElement).toBe(focused);
    expect(textOf(dialog)).toBe(asked(kind, 'Hades', 12, bytesSize(2 * GB)));
  });

  it('closes when the group has gone, sends nothing, and hands focus on by the row rule', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 2 * GB }), group('g-c', 'Celeste', 3, { sizeBytes: GB })]))
      .mockResolvedValue(withGroups(kind, [group('g-c', 'Celeste', 3, { sizeBytes: GB })]));
    await openFor(kind, 'Hades');

    libraryChanges();
    await settle();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(kind.service).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(within(rowOf(kind.section, 'Celeste')).getByRole('button', { name: msg('diskusage.deleteAll') }));
  });

  it('after a partial delete asks with what is left, and Retry deletes that', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(withGroups(kind, [group('g-h', 'Hades', 12, { sizeBytes: 2 * GB })]))
      .mockResolvedValue(withGroups(kind, [group('g-h', 'Hades', 2, { sizeBytes: GB })]));
    vi.mocked(kind.service)
      .mockResolvedValueOnce({ deleted: 10, gone: 0, failed: 2, code: 'diskusage.delete_partial' })
      .mockResolvedValueOnce({ deleted: 2, gone: 0, failed: 0, code: '' });
    const { user, dialog } = await openFor(kind, 'Hades');

    await user.click(within(dialog).getByRole('button', { name: msg(kind.confirm) }));
    await vi.waitFor(() => expect(within(dialog).getByRole('alert').textContent?.trim()).toBe(msg('diskusage.delete_partial', { deleted: 10, failed: 2 })));
    expect(textOf(dialog)).toBe(asked(kind, 'Hades', 2, bytesSize(GB)));

    await user.click(within(dialog).getByRole('button', { name: msg('common.retry') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(kind.service).mock.calls).toEqual([['g-h'], ['g-h']]);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg(kind.done), 'success']]);
  });
});

describe('DiskUsage: the question of a single row', () => {
  it('keeps the path and the id that were pressed, whatever a rescan shows meanwhile', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ downloads: [download('d-1', 'Celeste torrent')], leftovers: [leftover('D:\\Games\\.partial', 'install_partial')] }))
      .mockResolvedValue(report({ downloads: [download('d-2', 'Other torrent')], leftovers: [leftover('D:\\Games\\.other', 'install_partial')] }));
    vi.mocked(deleteLeftover).mockResolvedValue();
    vi.mocked(deleteDownloadData).mockResolvedValue();
    const { user } = renderDom(DiskUsage);
    await screen.findByText('D:\\Games\\.partial');
    await settle();

    await user.click(within(rowOf('diskusage.leftoversTitle', 'D:\\Games\\.partial')).getByRole('button', { name: msg('diskusage.leftoverDelete') }));
    let dialog = await screen.findByRole('dialog');
    libraryChanges();
    await settle();
    expect(screen.queryByText('D:\\Games\\.other')).not.toBeNull();
    expect(screen.getByRole('dialog')).toBe(dialog);
    await user.click(within(dialog).getByRole('button', { name: msg('diskusage.leftoverDelete') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(deleteLeftover).mock.calls).toEqual([['D:\\Games\\.partial']]);

    vi.mocked(scanUsage).mockResolvedValueOnce(report({ downloads: [download('d-1', 'Celeste torrent')] }));
    libraryChanges();
    await screen.findByText('Celeste torrent');
    await settle();
    await user.click(within(rowOf('diskusage.downloadsTitle', 'Celeste torrent')).getByRole('button', { name: msg('diskusage.downloadDelete') }));
    dialog = await screen.findByRole('dialog');
    libraryChanges();
    await settle();
    expect(screen.queryByText('Other torrent')).not.toBeNull();
    await user.click(within(dialog).getByRole('button', { name: msg('diskusage.downloadDelete') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(deleteDownloadData).mock.calls).toEqual([['d-1']]);
  });
});
