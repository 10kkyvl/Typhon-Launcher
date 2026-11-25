import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import {
  cachedUsage,
  deleteDownloadData,
  deleteLeftover,
  deleteScreenshots,
  deleteSnapshots,
  scanUsage,
  type Report,
} from '../../lib/services/diskusage';
import { inspectRemoval, removeGame } from '../../lib/services/install';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames } from '../../lib/stores/library';
import { moveTarget } from '../../lib/stores/relocate';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import DiskUsage from './DiskUsage.svelte';

// The page keeps the report, the sort and the one confirmation, and hands them down. A callback not
// wired, a total read from the wrong field or a kind given the other kind's words breaks nothing a
// type check sees: these tests go through such seams the way a player does: each section's Delete
// to the page's confirmation and the service call it ends in with its argument, then the rescan; the
// sort menu to the order of the games; Remove to the page's dialog for that game and its rescan;
// Move to the move store; each section's total from its own field; the warnings. Not held here: the
// reason lines and marks of a row, disabled buttons and their reasons, the note on unknown sizes,
// the busy Rescan, the scan error's notice, the `emulated` flag the remove dialog gets from the
// library store, the error text of a failed delete; of a confirmation: its title, note and button
// words, the toast after it, that the dialog stays until the delete and the rescan are over; an
// overtaken scan's answer dropped, a cached report never replacing a newer scan; the games section
// hidden with no games; a section's hint (`DiskUsage.lifecycle.dom.test.ts` holds what the page reads
// on mount and when a move ends).

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  deleteDownloadData: vi.fn(),
  deleteLeftover: vi.fn(),
  deleteScreenshots: vi.fn(),
  deleteSnapshots: vi.fn(),
}));
vi.mock('../../lib/services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/install')>()),
  inspectRemoval: vi.fn(),
  removeGame: vi.fn(),
}));

const GB = 1_000_000_000;
const game = (id: string, title: string, sizeBytes: number, lastPlayed: string | null) => ({
  id,
  title,
  installDir: `C:\\Games\\${title}`,
  sizeBytes,
  sizeUnknown: false,
  missing: false,
  external: false,
  linked: false,
  lastPlayed,
  installedAt: '2026-01-01T00:00:00Z',
  running: false,
  busy: false,
});
const group = (gameId: string, title: string, count: number) => ({ gameId, title, inLibrary: true, count, sizeBytes: GB, sizeUnknown: false });

const report = {
  games: [game('g-hades', 'Hades', 9 * GB, '2026-11-01T10:00:00Z'), game('g-celeste', 'Celeste', 2 * GB, null)],
  downloads: [
    { id: 'd-1', name: 'Celeste torrent', root: 'D:\\Downloads\\Celeste', sizeBytes: GB, sizeUnknown: false, missing: false, seeding: false, installed: true, gameId: 'g-celeste', referenced: false, reason: '', reasonCode: '' },
  ],
  leftovers: [{ path: 'D:\\Games\\.partial', kind: 'install_partial', sizeBytes: GB, sizeUnknown: false, deletable: true, reason: '', reasonCode: '' }],
  screenshots: [group('g-hades', 'Hades', 12)],
  snapshots: [group('g-celeste', 'Celeste', 4)],
  warnings: [
    { area: 'leftovers', code: 'diskusage.warning.folder_unreadable', message: 'denied' },
    { area: 'leftovers', code: 'diskusage.warning.from_a_newer_go', message: 'denied' },
  ],
  totals: { gamesBytes: 11 * GB, downloadsBytes: 21 * GB, leftoversBytes: 31 * GB, screenshotsBytes: 41 * GB, snapshotsBytes: 51 * GB, totalBytes: 99 * GB, reclaimableBytes: 61 * GB, unknownSizes: false },
  scannedAt: '2026-11-12T00:00:00Z',
} as Report;

const section = (title: string) => screen.getByRole('heading', { level: 2, name: title }).closest('section') as HTMLElement;
const rowTitles = (title: string) => [...section(title).querySelectorAll('.row .title')].map((el) => el.textContent);

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(scanUsage).mockResolvedValue(report);
  for (const remove of [deleteDownloadData, deleteLeftover]) vi.mocked(remove).mockResolvedValue();
  for (const remove of [deleteScreenshots, deleteSnapshots]) vi.mocked(remove).mockResolvedValue({ deleted: 1, gone: 0, failed: 0, code: '' });
  libraryGames.set([{ id: 'g-hades', title: 'Hades' }, { id: 'g-celeste', title: 'Celeste' }] as LibraryGame[]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
  moveTarget.set(null);
});

async function shown() {
  const view = renderDom(DiskUsage);
  await screen.findByRole('heading', { level: 2, name: msg('diskusage.gamesTitle') });
  return view;
}

describe('DiskUsage: a Delete in each section', () => {
  it.each([
    ['diskusage.downloadsTitle', 'Celeste torrent', 'diskusage.downloadDelete', msg('diskusage.downloadConfirmText', { name: 'Celeste torrent' }), deleteDownloadData, 'd-1'],
    ['diskusage.leftoversTitle', msg('diskusage.kindInstallPartial'), 'diskusage.leftoverDelete', msg('diskusage.leftoverConfirmText', { kind: msg('diskusage.kindInstallPartial'), path: 'D:\\Games\\.partial' }), deleteLeftover, 'D:\\Games\\.partial'],
    ['diskusage.screenshotsTitle', 'Hades', 'diskusage.deleteAll', msg('diskusage.screenshotsConfirmText', { title: 'Hades', count: 12 }), deleteScreenshots, 'g-hades'],
    ['diskusage.snapshotsTitle', 'Celeste', 'diskusage.deleteAll', msg('diskusage.snapshotsConfirmText', { title: 'Celeste', count: 4 }), deleteSnapshots, 'g-celeste'],
  ] as const)('in %s asks with that row’s words, deletes it by its own argument and scans again', async (heading, title, button, text, service, argument) => {
    const { user } = await shown();
    const row = [...section(msg(heading)).querySelectorAll<HTMLElement>('.row')].find((el) => el.querySelector('.title')?.textContent === title) as HTMLElement;

    await user.click(within(row).getByRole('button', { name: msg(button) }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(text)).toBeTruthy();
    const buttons = within(dialog).getAllByRole('button');
    await user.click(buttons[buttons.length - 1]);

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(service).mock.calls).toEqual([[argument]]);
    for (const other of [deleteDownloadData, deleteLeftover, deleteScreenshots, deleteSnapshots].filter((s) => s !== service)) {
      expect(other).not.toHaveBeenCalled();
    }
    expect(scanUsage).toHaveBeenCalledTimes(2);
  });
});

describe('DiskUsage: the games section', () => {
  it('the sort picked in its menu reorders the games and is said on its chip', async () => {
    const { user } = await shown();
    const chip = (sort: string) => msg('diskusage.sortLabel', { sort });
    expect(rowTitles(msg('diskusage.gamesTitle'))).toEqual(['Hades', 'Celeste']);

    await user.click(screen.getByRole('button', { name: chip(msg('diskusage.sortSize')) }));
    await user.click(screen.getByRole('menuitemradio', { name: msg('diskusage.sortPlayed') }));

    expect(rowTitles(msg('diskusage.gamesTitle'))).toEqual(['Celeste', 'Hades']);
    expect(screen.getByRole('button', { name: chip(msg('diskusage.sortPlayed')) })).toBeTruthy();
  });

  it('Remove opens the page’s dialog for that game, and a removal scans again', async () => {
    vi.mocked(inspectRemoval).mockResolvedValue({ owned: true, dirMissing: false, method: 'files', running: false, busy: false } as Awaited<ReturnType<typeof inspectRemoval>>);
    vi.mocked(removeGame).mockResolvedValue();
    const { user } = await shown();
    const row = [...section(msg('diskusage.gamesTitle')).querySelectorAll<HTMLElement>('.row')].find((el) => el.querySelector('.title')?.textContent === 'Celeste') as HTMLElement;

    await user.click(within(row).getByRole('button', { name: msg('diskusage.remove') }));

    const dialog = await screen.findByRole('dialog');
    expect(inspectRemoval).toHaveBeenCalledWith('g-celeste');
    const chosen = within(dialog).getAllByRole('radio').filter((radio) => radio.getAttribute('aria-checked') === 'true');
    expect(chosen.map((radio) => radio.querySelector('.mode-label')?.textContent)).toEqual([msg('modals.removeGameModeDiskLabel')]);
    const buttons = within(dialog).getAllByRole('button');
    await vi.waitFor(() => expect(buttons[buttons.length - 1].matches(':disabled')).toBe(false));
    await user.click(buttons[buttons.length - 1]);

    await vi.waitFor(() => expect(scanUsage).toHaveBeenCalledTimes(2));
    expect(vi.mocked(removeGame).mock.calls.map((call) => call[0])).toEqual(['g-celeste']);
  });

  it('Move asks the move store for that game', async () => {
    const { user } = await shown();
    const row = [...section(msg('diskusage.gamesTitle')).querySelectorAll<HTMLElement>('.row')].find((el) => el.querySelector('.title')?.textContent === 'Hades') as HTMLElement;

    await user.click(within(row).getByRole('button', { name: msg('diskusage.move') }));

    expect(get(moveTarget)).toEqual({ gameId: 'g-hades', title: 'Hades' });
  });
});

describe('DiskUsage: a reason Go gives in a code the table does not know', () => {
  it('is said in the row’s plain words, never in Go’s text', async () => {
    vi.mocked(scanUsage).mockResolvedValue({
      ...report,
      leftovers: [{ path: 'D:\\Games\\.odd', kind: 'install_partial', sizeBytes: GB, sizeUnknown: false, deletable: false, reason: 'текст из Go', reasonCode: 'diskusage.reason.from_a_newer_go' }],
      downloads: [{ ...report.downloads[0], referenced: true, reason: 'текст из Go', reasonCode: 'diskusage.reason.from_a_newer_go' }],
    });
    await shown();
    for (const title of ['diskusage.leftoversTitle', 'diskusage.downloadsTitle'] as const) {
      const box = section(msg(title));
      expect(within(box).getByText(msg('diskusage.cannotDelete')), title).toBeTruthy();
      expect(box.textContent, title).not.toContain('текст из Go');
    }
  });
});

describe('DiskUsage: what each part is given', () => {
  it('each section shows the total of its own field', async () => {
    await shown();
    const headings = ['diskusage.gamesTitle', 'diskusage.downloadsTitle', 'diskusage.leftoversTitle', 'diskusage.screenshotsTitle', 'diskusage.snapshotsTitle'] as const;
    const totals = headings.map((key) => section(msg(key)).querySelector('.section-total')?.textContent);
    expect(totals).toEqual([11, 21, 31, 41, 51].map((n) => bytesSize(n * GB)));
  });

  it('the save backups and the screenshots each count their own files', async () => {
    await shown();
    expect(within(section(msg('diskusage.screenshotsTitle'))).getByText(msg('diskusage.screenshotsCount', { count: 12 }))).toBeTruthy();
    expect(within(section(msg('diskusage.snapshotsTitle'))).getByText(msg('diskusage.snapshotsCount', { count: 4 }))).toBeTruthy();
  });

  it('the warnings of the report are listed in their catalogued words, never in Go’s', async () => {
    await shown();
    const list = screen.getByRole('heading', { level: 2, name: msg('diskusage.warningsTitle') }).closest('.notice') as HTMLElement;
    expect([...list.querySelectorAll('li')].map((li) => li.textContent)).toEqual([msg('diskusage.warning.folder_unreadable'), msg('diskusage.warningUnknown')]);
    expect(list.textContent).not.toContain('denied');
    expect(list.textContent).not.toContain('leftovers');
  });
});
