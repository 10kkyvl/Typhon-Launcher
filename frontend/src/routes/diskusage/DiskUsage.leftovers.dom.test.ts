import { cleanup, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg, type MessageKey } from '../../lib/i18n';
import { cachedUsage, deleteLeftover, scanUsage } from '../../lib/services/diskusage';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames } from '../../lib/stores/library';
import { renderDom } from '../../lib/testing/dom';
import DiskUsage from './DiskUsage.svelte';
import { leftover, report, rowOf } from './diskUsageTesting';

// The leftovers Go added to the report: copies a move left, unfinished removals, staging folders no
// journal names. None is deletable by Go's answer, and the screen offers no delete for them; each says
// what it is, whose game it is and why it stays. A folder that may hold the only copy of a game, or a
// game itself, reads as a warning. The services are mocked whole: nothing is deleted.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/diskusage', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/diskusage')>()),
  cachedUsage: vi.fn(),
  scanUsage: vi.fn(),
  cancelScan: vi.fn(),
  deleteLeftover: vi.fn(),
}));

const kept = (path: string, kind: string, reasonCode: string, patch = {}) =>
  leftover(path, kind, { deletable: false, reason: 'текст из Go', reasonCode, ...patch });

const ROWS = [
  kept('E:\\Games\\Hades', 'relocate_copy_target', 'diskusage.reason.only_copy', { gameId: 'g-hades', title: 'Hades' }),
  kept('D:\\Games\\Old', 'relocate_copy_source', 'diskusage.reason.relocate_copy', { gameId: 'g-old', title: 'Old Game' }),
  kept('E:\\Games\\Doom.typhon-staging', 'relocate_copy_staging', 'diskusage.reason.holds_game', { gameId: 'g-x' }),
  kept('D:\\Games\\Quake.removing', 'removal', 'diskusage.reason.removal_pending'),
  kept('D:\\Games\\Myst.removing-2', 'removal', 'diskusage.reason.removal_unjournaled'),
  kept('D:\\Games\\Riven.removing', 'removal', 'diskusage.reason.removals_unreadable'),
  kept('D:\\Games\\Tomb.typhon-staging', 'relocate_staging', 'diskusage.reason.relocate_unjournaled'),
  kept('D:\\Games\\Held', 'install_partial', 'diskusage.reason.folder_held'),
];

beforeEach(() => {
  vi.mocked(cachedUsage).mockResolvedValue(null);
  vi.mocked(scanUsage).mockResolvedValue(report({ leftovers: ROWS }));
  libraryGames.set([{ id: 'g-hades', title: 'Hades' }] as LibraryGame[]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  libraryGames.set([]);
});

async function shown() {
  renderDom(DiskUsage);
  await screen.findByText('E:\\Games\\Hades');
}

const game = (row: HTMLElement) => row.querySelector('.game')?.textContent?.replace(/\s+/g, ' ').trim();

describe('DiskUsage: what a new leftover row says', () => {
  it.each<[string, MessageKey, string | null, MessageKey]>([
    ['E:\\Games\\Hades', 'diskusage.kindRelocateCopyTarget', 'Hades', 'diskusage.reason.only_copy'],
    ['D:\\Games\\Old', 'diskusage.kindRelocateCopySource', 'Old Game', 'diskusage.reason.relocate_copy'],
    ['E:\\Games\\Doom.typhon-staging', 'diskusage.kindRelocateCopyStaging', null, 'diskusage.reason.holds_game'],
    ['D:\\Games\\Quake.removing', 'diskusage.kindRemoval', null, 'diskusage.reason.removal_pending'],
    ['D:\\Games\\Myst.removing-2', 'diskusage.kindRemoval', null, 'diskusage.reason.removal_unjournaled'],
    ['D:\\Games\\Riven.removing', 'diskusage.kindRemoval', null, 'diskusage.reason.removals_unreadable'],
  ])('%s: its kind, whose game it is and why it stays, with no delete', async (path, kind, title, reason) => {
    await shown();
    const row = rowOf('diskusage.leftoversTitle', path);
    expect(row.querySelector('.title')?.textContent?.trim()).toBe(msg(kind));
    expect(game(row)).toBe(msg('diskusage.copyOf', { title: title ?? msg('diskusage.unknownGame') }));
    expect(within(row).getByText(msg(reason))).toBeTruthy();
    expect(within(row).queryByRole('button', { name: msg('diskusage.leftoverDelete') })).toBeNull();
    expect(row.textContent).not.toContain('текст из Go');
  });

  it('leads to the page of a game the library holds, and names one it does not as plain text', async () => {
    await shown();
    expect(within(rowOf('diskusage.leftoversTitle', 'E:\\Games\\Hades')).getByRole('button', { name: 'Hades' })).toBeTruthy();
    expect(within(rowOf('diskusage.leftoversTitle', 'D:\\Games\\Old')).queryByRole('button', { name: 'Old Game' })).toBeNull();
  });

  it('says a folder that may be the only copy of a game, or holds one, as a warning, and the rest plainly', async () => {
    await shown();
    const tone = (path: string) => rowOf('diskusage.leftoversTitle', path).querySelector('.reason')?.getAttribute('data-tone');
    expect(tone('E:\\Games\\Hades')).toBe('warning');
    expect(tone('E:\\Games\\Doom.typhon-staging')).toBe('warning');
    for (const path of ['D:\\Games\\Old', 'D:\\Games\\Quake.removing', 'D:\\Games\\Tomb.typhon-staging', 'D:\\Games\\Held']) {
      expect(tone(path), path).toBe('plain');
    }
  });

  it('offers no delete on any of them, so nothing reaches Go', async () => {
    await shown();
    const list = rowOf('diskusage.leftoversTitle', 'E:\\Games\\Hades').closest('ul') as HTMLElement;
    expect(within(list).queryAllByRole('button').map((b) => b.textContent?.trim())).toEqual(['Hades']);
    expect(deleteLeftover).not.toHaveBeenCalled();
  });

  it('keeps the older kinds without a game line', async () => {
    await shown();
    expect(game(rowOf('diskusage.leftoversTitle', 'D:\\Games\\Tomb.typhon-staging'))).toBeUndefined();
    expect(within(rowOf('diskusage.leftoversTitle', 'D:\\Games\\Held')).getByText(msg('diskusage.reason.folder_held'))).toBeTruthy();
  });
});
