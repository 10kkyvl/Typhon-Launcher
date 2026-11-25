import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { cachedUsage, deleteLeftover, scanUsage, type Report } from '../../lib/services/diskusage';
import { inspectRemoval, removeGame } from '../../lib/services/install';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { renderDom } from '../../lib/testing/dom';
import DiskUsage from './DiskUsage.svelte';
import { game, group, leftover, report, rowOf } from './diskUsageTesting';

// Where focus goes when the row that holds it, or the row a dialog returns to, leaves or loses its
// control: a refused delete whose row stays without Delete, a rescan the page did not start, the
// removal dialog closing onto a game the rescan then drops. Focus anywhere else is left alone. The
// services are mocked whole: nothing is measured, removed or deleted.

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
vi.mock('../../lib/services/install', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/install')>()),
  inspectRemoval: vi.fn(),
  removeGame: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((yes) => (resolve = yes));
  return { promise, resolve };
}

async function settle() {
  for (let i = 0; i < 6; i++) {
    await tick();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

const A = 'D:\\Games\\.a';
const B = 'D:\\Games\\.b';
const C = 'D:\\Games\\.c';
const partial = (path: string, patch = {}) => leftover(path, 'install_partial', patch);
const deleteOf = (path: string) => within(rowOf('diskusage.leftoversTitle', path)).getByRole('button', { name: msg('diskusage.leftoverDelete') });

const INSTALLED = [{ id: 'g1', title: 'One', installDir: 'C:\\Games\\One' }] as LibraryGame[];
// A game leaves the installed games: the page measures again on its own (DiskUsage.svelte).
const libraryChanges = () => libraryGames.set([{ ...INSTALLED[0], uninstalled: true }] as LibraryGame[]);

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  libraryGames.set(INSTALLED);
  libraryLoaded.set(true);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
  libraryLoaded.set(false);
  document.querySelectorAll('[data-outside]').forEach((el) => el.remove());
});

describe('DiskUsage: focus when a refused delete leaves its row without Delete', () => {
  it('goes to the same control of the row below once the confirmation closes', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 }), partial(B, { sizeBytes: 2 })] }))
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3, deletable: false, reasonCode: 'diskusage.reason.folder_held' }), partial(B, { sizeBytes: 2 })] }));
    vi.mocked(deleteLeftover).mockRejectedValue(new Error('typhon:diskusage.not_deletable: x'));
    const { user } = renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();
    await user.click(deleteOf(A));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('diskusage.leftoverDelete') }));
    await vi.waitFor(() => expect(within(dialog).getByRole('alert').textContent?.trim()).toBe(msg('errInstall.diskusageNotDeletable')));
    await settle();

    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await settle();
    expect(document.activeElement).toBe(deleteOf(B));
  });
});

describe('DiskUsage: focus when a rescan the page did not start drops the focused row', () => {
  it('goes to the same control of the row below', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 }), partial(B, { sizeBytes: 2 })] }))
      .mockResolvedValueOnce(report({ leftovers: [partial(B, { sizeBytes: 2 })] }));
    renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();
    deleteOf(A).focus();

    libraryChanges();
    await settle();
    expect(scanUsage).toHaveBeenCalledTimes(2);
    expect(screen.queryByText(A)).toBeNull();
    expect(document.activeElement).toBe(deleteOf(B));
  });

  it('goes to the row above when the last row went', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 }), partial(B, { sizeBytes: 2 }), partial(C, { sizeBytes: 1 })] }))
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 }), partial(B, { sizeBytes: 2 })] }));
    renderDom(DiskUsage);
    await screen.findByText(C);
    await settle();
    deleteOf(C).focus();

    libraryChanges();
    await settle();
    expect(document.activeElement).toBe(deleteOf(B));
  });

  it('keeps to the row’s own section: the row above it there, not a row of the section below', async () => {
    const shots = report({ screenshots: [group('g-h', 'Hades', 4)] }).screenshots;
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 }), partial(B, { sizeBytes: 2 })], screenshots: shots }))
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 })], screenshots: shots }));
    renderDom(DiskUsage);
    await screen.findByText(B);
    await settle();
    deleteOf(B).focus();

    libraryChanges();
    await settle();
    expect(screen.queryByText(B)).toBeNull();
    expect(document.activeElement).toBe(deleteOf(A));
  });

  it('goes to the first control left in the section when no row of it has one', async () => {
    libraryGames.set([...INSTALLED, { id: 'g-hades', title: 'Hades', installDir: 'C:\\Games\\Hades' }] as LibraryGame[]);
    const kept = leftover('E:\\Games\\Hades', 'relocate_copy_target', { gameId: 'g-hades', title: 'Hades', deletable: false, reasonCode: 'diskusage.reason.only_copy', sizeBytes: 1 });
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 }), kept] }))
      .mockResolvedValueOnce(report({ leftovers: [kept] }));
    renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();
    deleteOf(A).focus();

    libraryGames.set([{ ...INSTALLED[0], uninstalled: true }, { id: 'g-hades', title: 'Hades', installDir: 'C:\\Games\\Hades' }] as LibraryGame[]);
    await settle();
    expect(screen.queryByText(A)).toBeNull();
    expect(document.activeElement).toBe(within(rowOf('diskusage.leftoversTitle', 'E:\\Games\\Hades')).getByRole('button', { name: 'Hades' }));
  });

  it('leaves focus that is elsewhere alone: the page’s own controls, a field outside the page, a dialog', async () => {
    vi.mocked(scanUsage)
      .mockResolvedValueOnce(report({ leftovers: [partial(A, { sizeBytes: 3 }), partial(B, { sizeBytes: 2 }), partial(C, { sizeBytes: 1 })] }))
      .mockResolvedValueOnce(report({ leftovers: [partial(B, { sizeBytes: 2 }), partial(C, { sizeBytes: 1 })] }))
      .mockResolvedValueOnce(report({ leftovers: [partial(C, { sizeBytes: 1 })] }))
      .mockResolvedValueOnce(report({ leftovers: [] }));
    const { user } = renderDom(DiskUsage);
    await screen.findByText(A);
    await settle();

    const rescan = screen.getByRole('button', { name: msg('diskusage.rescan') });
    rescan.focus();
    libraryChanges();
    await settle();
    expect(screen.queryByText(A)).toBeNull();
    expect(document.activeElement).toBe(rescan);

    const search = document.createElement('input');
    search.setAttribute('data-outside', '');
    search.setAttribute('aria-label', 'search');
    document.body.append(search);
    search.focus();
    libraryGames.set(INSTALLED);
    await settle();
    expect(screen.queryByText(B)).toBeNull();
    expect(document.activeElement).toBe(search);

    await user.click(deleteOf(C));
    const dialog = await screen.findByRole('dialog');
    const inDialog = document.activeElement;
    expect(dialog.contains(inDialog)).toBe(true);
    libraryChanges();
    await settle();
    expect(screen.queryByText(C)).toBeNull();
    expect(document.activeElement).toBe(inDialog);
  });
});

describe('DiskUsage: focus after the removal dialog', () => {
  const ALPHA_BETA = [
    { id: 'g-a', title: 'Alpha', installDir: 'C:\\Games\\Alpha' },
    { id: 'g-b', title: 'Beta', installDir: 'C:\\Games\\Beta' },
  ] as LibraryGame[];
  const both = () => report({ games: [game('g-a', 'Alpha', { sizeBytes: 3 }), game('g-b', 'Beta', { sizeBytes: 2 })] });
  const betaOnly = () => report({ games: [game('g-b', 'Beta', { sizeBytes: 2 })] });
  const removeOf = (title: string) => within(rowOf('diskusage.gamesTitle', title)).getByRole('button', { name: msg('diskusage.remove') });

  beforeEach(() => {
    libraryGames.set(ALPHA_BETA);
    vi.mocked(inspectRemoval).mockResolvedValue({
      gameId: 'g-a', title: 'Alpha', installDir: 'C:\\Games\\Alpha', installType: 'repack', method: 'files', owned: true,
      uninstallUnknown: false, sizeBytes: 3, sizeUnknown: false, dirMissing: false, quietUninstall: false, running: false, busy: false,
      downloadId: '', downloadPresent: false, downloadSeeding: false,
    } as never);
  });

  async function removeAlpha(user: ReturnType<typeof renderDom>['user']) {
    await user.click(removeOf('Alpha'));
    const dialog = await screen.findByRole('dialog');
    const confirm = () => within(dialog).getByRole('button', { name: msg('modals.removeGameModeDiskLabel') });
    await vi.waitFor(() => expect(confirm().matches(':disabled')).toBe(false));
    await user.click(confirm());
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  }

  it('goes on by the rule when the rescan drops the removed game after the dialog closed', async () => {
    const rescan = deferred<Report>();
    vi.mocked(scanUsage).mockResolvedValueOnce(both()).mockReturnValueOnce(rescan.promise);
    vi.mocked(removeGame).mockResolvedValue(undefined as never);
    const { user } = renderDom(DiskUsage);
    await screen.findByText('Beta');
    await settle();

    await removeAlpha(user);
    await settle();
    expect(document.activeElement).toBe(removeOf('Alpha'));

    rescan.resolve(betaOnly());
    await settle();
    expect(screen.queryByText('Alpha')).toBeNull();
    expect(document.activeElement).toBe(removeOf('Beta'));
  });

  it('goes on by the rule when the removed game left while the dialog was still open', async () => {
    vi.mocked(scanUsage).mockResolvedValueOnce(both()).mockResolvedValue(betaOnly());
    // The library tells of the removal before Go answers: the page measures again under the dialog.
    vi.mocked(removeGame).mockImplementation(async () => {
      libraryGames.set([{ ...ALPHA_BETA[0], uninstalled: true }, ALPHA_BETA[1]] as LibraryGame[]);
      await settle();
      expect(screen.queryByText('Alpha')).toBeNull();
      return undefined as never;
    });
    const { user } = renderDom(DiskUsage);
    await screen.findByText('Beta');
    await settle();

    await removeAlpha(user);
    await settle();
    expect(document.activeElement).toBe(removeOf('Beta'));
  });
});
