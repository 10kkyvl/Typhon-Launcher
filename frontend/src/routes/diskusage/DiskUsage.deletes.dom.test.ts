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
import { download, group, leftover, report, rowOf } from './diskUsageTesting';

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
    text: msg('diskusage.screenshotsConfirmText', { title: 'Hades', count: 12 }),
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
    text: msg('diskusage.snapshotsConfirmText', { title: 'Celeste', count: 4 }),
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
});
