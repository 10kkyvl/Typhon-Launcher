import { cleanup, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../../lib/i18n';
import {
  cachedUsage,
  deleteDownloadData,
  deleteLeftover,
  deleteScreenshots,
  deleteSnapshots,
  scanUsage,
} from '../../lib/services/diskusage';
import { toast } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import DiskUsage from './DiskUsage.svelte';
import { download, group, groupAsked, leftover, report, rowOf } from './diskUsageTesting';

// Each delete of the page, through its confirmation: the words it asks with, the one service call and
// its argument, what the player is told after it worked and after it failed. The services are mocked
// whole: nothing is deleted.

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

const shown = report({
  downloads: [download('d-1', 'Celeste torrent')],
  leftovers: [leftover('D:\\Games\\.partial', 'install_partial')],
  screenshots: [group('g-hades', 'Hades', 12)],
  snapshots: [group('g-celeste', 'Celeste', 4)],
});

const SERVICES = [deleteDownloadData, deleteLeftover, deleteScreenshots, deleteSnapshots] as const;

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(scanUsage).mockResolvedValue(shown);
  vi.mocked(deleteDownloadData).mockResolvedValue();
  vi.mocked(deleteLeftover).mockResolvedValue();
  vi.mocked(deleteScreenshots).mockResolvedValue({ deleted: 12, gone: 0, failed: 0, code: '' });
  vi.mocked(deleteSnapshots).mockResolvedValue({ deleted: 4, gone: 0, failed: 0, code: '' });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

interface Case {
  name: string;
  section: MessageKey;
  row: string;
  button: MessageKey;
  title: MessageKey;
  text: string;
  confirm: MessageKey;
  service: (typeof SERVICES)[number];
  argument: string;
  done: MessageKey;
}

const CASES: Case[] = [
  {
    name: 'download data',
    section: 'diskusage.downloadsTitle',
    row: 'Celeste torrent',
    button: 'diskusage.downloadDelete',
    title: 'diskusage.downloadConfirmTitle',
    text: msg('diskusage.downloadConfirmText', { name: 'Celeste torrent' }),
    confirm: 'diskusage.downloadDelete',
    service: deleteDownloadData,
    argument: 'd-1',
    done: 'diskusage.downloadDeleted',
  },
  {
    name: 'a leftover',
    section: 'diskusage.leftoversTitle',
    row: 'D:\\Games\\.partial',
    button: 'diskusage.leftoverDelete',
    title: 'diskusage.leftoverConfirmTitle',
    text: msg('diskusage.leftoverConfirmText', { kind: msg('diskusage.kindInstallPartial'), path: 'D:\\Games\\.partial' }),
    confirm: 'diskusage.leftoverDelete',
    service: deleteLeftover,
    argument: 'D:\\Games\\.partial',
    done: 'diskusage.leftoverDeleted',
  },
  {
    name: 'screenshots',
    section: 'diskusage.screenshotsTitle',
    row: 'Hades',
    button: 'diskusage.deleteAll',
    title: 'diskusage.screenshotsConfirmTitle',
    text: groupAsked('screenshots', 'Hades', 12),
    confirm: 'diskusage.deleteAll',
    service: deleteScreenshots,
    argument: 'g-hades',
    done: 'diskusage.screenshotsDeleted',
  },
  {
    name: 'save snapshots',
    section: 'diskusage.snapshotsTitle',
    row: 'Celeste',
    button: 'diskusage.deleteAll',
    title: 'diskusage.snapshotsConfirmTitle',
    text: groupAsked('snapshots', 'Celeste', 4),
    confirm: 'diskusage.snapshotsConfirmAction',
    service: deleteSnapshots,
    argument: 'g-celeste',
    done: 'diskusage.snapshotsDeleted',
  },
];

async function ask(c: Case) {
  const view = renderDom(DiskUsage);
  await screen.findByText(c.row);
  await view.user.click(within(rowOf(c.section, c.row)).getByRole('button', { name: msg(c.button) }));
  const dialog = await screen.findByRole('dialog', { name: msg(c.title) });
  return { ...view, dialog };
}

describe.each(CASES.map((c) => [c.name, c] as const))('DiskUsage: deleting %s', (_name, c) => {
  it('asks in the row’s words and deletes nothing before the confirmation', async () => {
    const { dialog } = await ask(c);
    expect(within(dialog).getByText(c.text)).toBeTruthy();
    expect(within(dialog).getByRole('button', { name: msg(c.confirm) })).toBeTruthy();
    for (const service of SERVICES) expect(service).not.toHaveBeenCalled();
  });

  it('Cancel closes and deletes nothing', async () => {
    const { user, dialog } = await ask(c);
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    for (const service of SERVICES) expect(service).not.toHaveBeenCalled();
  });

  it('deletes by the row’s own argument, says it worked and measures again', async () => {
    const { user, dialog } = await ask(c);
    await user.click(within(dialog).getByRole('button', { name: msg(c.confirm) }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(c.service).mock.calls).toEqual([[c.argument]]);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg(c.done), 'success']]);
    expect(scanUsage).toHaveBeenCalledTimes(2);
  });

  it('when Go refuses, says why in the catalogued words', async () => {
    vi.mocked(c.service).mockRejectedValue(new Error('typhon:diskusage.not_scanned: текст из Go'));
    const { user, dialog } = await ask(c);
    await user.click(within(dialog).getByRole('button', { name: msg(c.confirm) }));
    await vi.waitFor(() => expect(scanUsage).toHaveBeenCalledTimes(2));
    expect(document.body.textContent).not.toContain('текст из Go');
    const said = [...vi.mocked(toast).mock.calls, [screen.queryByRole('dialog')?.textContent ?? '']].flat().join(' ');
    expect(said).toContain(msg('errInstall.diskusageNotScanned'));
  });

  it('when Go refuses, keeps the failure in the confirmation, with Retry, and re-reads the disk', async () => {
    vi.mocked(c.service).mockRejectedValue(new Error('typhon:diskusage.not_deletable: текст из Go'));
    const { user, dialog } = await ask(c);
    await user.click(within(dialog).getByRole('button', { name: msg(c.confirm) }));

    await vi.waitFor(() => expect(within(dialog).getByRole('alert').textContent?.trim()).toBe(msg('errInstall.diskusageNotDeletable')));
    expect(screen.getByRole('dialog', { name: msg(c.title) })).toBe(dialog);
    expect(within(dialog).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(toast).not.toHaveBeenCalled();
    expect(scanUsage).toHaveBeenCalledTimes(2);

    vi.mocked(c.service).mockReset();
    const worked = c.service === deleteScreenshots || c.service === deleteSnapshots ? { deleted: 1, gone: 0, failed: 0, code: '' as const } : undefined;
    (vi.mocked(c.service) as unknown as { mockResolvedValue(value: unknown): void }).mockResolvedValue(worked);
    await user.click(within(dialog).getByRole('button', { name: msg('common.retry') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(c.service).mock.calls).toEqual([[c.argument]]);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg(c.done), 'success']]);
  });
});

describe.each([
  ['screenshots', 'diskusage.screenshotsTitle', 'Hades', 'diskusage.screenshotsConfirmTitle', 'diskusage.deleteAll', deleteScreenshots],
  ['save snapshots', 'diskusage.snapshotsTitle', 'Celeste', 'diskusage.snapshotsConfirmTitle', 'diskusage.snapshotsConfirmAction', deleteSnapshots],
] as const)('DiskUsage: a group of %s Go could delete only in part', (_name, heading, row, title, confirm, service) => {
  it('says what went and what stayed in the confirmation, and re-reads the disk', async () => {
    vi.mocked(service).mockResolvedValue({ deleted: 9, gone: 1, failed: 2, code: 'diskusage.delete_partial' });
    const view = renderDom(DiskUsage);
    await screen.findByText(row);
    await view.user.click(within(rowOf(heading, row)).getByRole('button', { name: msg('diskusage.deleteAll') }));
    const dialog = await screen.findByRole('dialog', { name: msg(title) });
    await view.user.click(within(dialog).getByRole('button', { name: msg(confirm) }));

    await vi.waitFor(() => expect(within(dialog).getByRole('alert').textContent?.trim()).toBe(msg('diskusage.delete_partial', { deleted: 10, failed: 2 })));
    expect(toast).not.toHaveBeenCalled();
    expect(scanUsage).toHaveBeenCalledTimes(2);
  });
});

describe('DiskUsage: a group of a game nothing knows', () => {
  it('is named "Unknown game" in its row and in its confirmation, never by an empty title or its id', async () => {
    vi.mocked(scanUsage).mockResolvedValue(report({ screenshots: [group('3f2a-raw-id', '', 3, { titleUnknown: true, inLibrary: false })] }));
    const view = renderDom(DiskUsage);
    await screen.findByText(msg('diskusage.screenshotsCount', { count: 3 }));
    const row = rowOf('diskusage.screenshotsTitle', msg('diskusage.unknownGame'));
    expect(row.textContent).not.toContain('3f2a-raw-id');
    await view.user.click(within(row).getByRole('button', { name: msg('diskusage.deleteAll') }));
    const dialog = await screen.findByRole('dialog', { name: msg('diskusage.screenshotsConfirmTitle') });
    expect(within(dialog).getByText(groupAsked('screenshots', msg('diskusage.unknownGame'), 3))).toBeTruthy();
  });
});

describe('DiskUsage: a row Go keeps', () => {
  it('offers no Delete, only its reason', async () => {
    vi.mocked(scanUsage).mockResolvedValue(
      report({
        downloads: [download('d-ref', 'Needed', { referenced: true, reasonCode: 'diskusage.reason.owned_by_install' })],
        leftovers: [leftover('D:\\Games\\.kept', 'update_previous', { deletable: false, reasonCode: 'diskusage.reason.update_owned' })],
      }),
    );
    renderDom(DiskUsage);
    await screen.findByText('Needed');
    const download_ = rowOf('diskusage.downloadsTitle', 'Needed');
    const kept = rowOf('diskusage.leftoversTitle', 'D:\\Games\\.kept');
    expect(within(download_).queryAllByRole('button')).toEqual([]);
    expect(within(kept).queryAllByRole('button')).toEqual([]);
    expect(within(download_).getByText(msg('diskusage.reason.owned_by_install'))).toBeTruthy();
    expect(within(kept).getByText(msg('diskusage.reason.update_owned'))).toBeTruthy();
  });
});

describe('DiskUsage: focus after a delete', () => {
  const three = report({
    leftovers: [
      leftover('D:\\Games\\.a', 'install_partial', { sizeBytes: 3 }),
      leftover('D:\\Games\\.b', 'install_partial', { sizeBytes: 2 }),
      leftover('D:\\Games\\.c', 'install_partial', { sizeBytes: 1 }),
    ],
  });
  const without = (path: string) => report({ leftovers: three.leftovers.filter((l) => l.path !== path) });

  async function deleteRow(path: string, after: ReturnType<typeof report>) {
    vi.mocked(scanUsage).mockResolvedValueOnce(three).mockResolvedValueOnce(after);
    const view = renderDom(DiskUsage);
    await screen.findByText(path);
    const button = within(rowOf('diskusage.leftoversTitle', path)).getByRole('button', { name: msg('diskusage.leftoverDelete') });
    await view.user.click(button);
    const dialog = await screen.findByRole('dialog');
    await view.user.click(within(dialog).getByRole('button', { name: msg('diskusage.leftoverDelete') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).not.toBe(document.body));
  }

  it('goes to the same control of the row that took the place of the one deleted', async () => {
    await deleteRow('D:\\Games\\.b', without('D:\\Games\\.b'));
    expect(document.activeElement).toBe(within(rowOf('diskusage.leftoversTitle', 'D:\\Games\\.c')).getByRole('button', { name: msg('diskusage.leftoverDelete') }));
  });

  it('goes to the row above when the last row went', async () => {
    await deleteRow('D:\\Games\\.c', without('D:\\Games\\.c'));
    expect(document.activeElement).toBe(within(rowOf('diskusage.leftoversTitle', 'D:\\Games\\.b')).getByRole('button', { name: msg('diskusage.leftoverDelete') }));
  });

  it('goes to the page heading when the section has nothing left to take it', async () => {
    await deleteRow('D:\\Games\\.a', report({ downloads: [download('d-1', 'Other')] }));
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });
});
