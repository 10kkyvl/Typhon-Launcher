import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import type { StorageInfo } from '../../lib/services/system';
import { tick } from 'svelte';
import type { MoveJob } from '../../lib/services/relocate';
import { getStorageInfo } from '../../lib/services/system';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { moves } from '../../lib/stores/relocate';
import { route } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { storageFailed, storageInfo } from '../../lib/stores/storage';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import Installed from './Installed.svelte';

// The storage card: the disk of the library folder, what the games on it take, the rest and the free space.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/compat', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/compat')>()),
  compatStatuses: vi.fn(async () => new Map()),
}));
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  getStorageInfo: vi.fn(() => new Promise(() => {})),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(),
}));
vi.mock('../../lib/stores/missingImages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/missingImages')>()),
  refreshMissingImages: vi.fn(async () => {}),
}));

const GB = 1024 ** 3;

function game(id: string, title: string, sizeBytes: number, extra: Partial<LibraryGame> = {}): LibraryGame {
  return { id, title, installDir: `C:\\Games\\${title}`, executable: 'game.exe', sizeBytes, uninstalled: false, tags: [], lastPlayed: null, ...extra } as unknown as LibraryGame;
}

const info: StorageInfo = { path: 'C:\\Games', volume: 'C:', filesystem: 'NTFS', totalBytes: 100 * GB, freeBytes: 40 * GB, usedBytes: 60 * GB };

beforeEach(() => {
  libraryLoaded.set(true);
  settings.set({ libraryPath: 'C:\\Games' } as NonNullable<Parameters<typeof settings.set>[0]>);
});

afterEach(() => {
  cleanup();
  libraryGames.set([]);
  libraryLoaded.set(false);
  settings.set(null);
  storageInfo.set(null);
  storageFailed.set(false);
  moves.set([]);
  Reflect.deleteProperty(Element.prototype, 'scrollIntoView');
  vi.clearAllMocks();
  document.body.replaceChildren();
});

// Each legend entry of the meter as one line of words.
const legend = () => [...document.querySelectorAll('.meter .legend > li')].map((item) => item.textContent?.replace(/\s+/g, ' ').trim());
const words = (key: Parameters<typeof msg>[0], size: number) => msg(key, { size: bytesSize(size) });

describe('Installed: the storage card', () => {
  it('draws the games on the library’s disk on the shared meter, largest first, with other files and the free space', () => {
    libraryGames.set([game('g-b', 'B', 5 * GB), game('g-a', 'A', 10 * GB), game('g-d', 'D', 7 * GB, { installDir: 'D:\\Other\\D' })]);
    storageInfo.set(info);
    renderDom(Installed);

    expect(screen.getByText(msg('games.installedStorageTitle'))).toBeTruthy();
    expect(screen.getByText(msg('games.installedStorageUsage', { used: bytesSize(60 * GB), total: bytesSize(100 * GB) }))).toBeTruthy();
    expect(legend()).toEqual([
      `A ${bytesSize(10 * GB)}`,
      `B ${bytesSize(5 * GB)}`,
      `${msg('common.meterOther')} ${bytesSize(45 * GB)}`,
      `${msg('common.meterFree')} ${bytesSize(40 * GB)}`,
      msg('common.meterTotal', { total: bytesSize(100 * GB) }),
    ]);
    const bar = screen.getByRole('group', { name: (name) => name.startsWith(msg('installed.meterDisk', { volume: 'C:' })) });
    expect([...bar.querySelectorAll('.part')].map((part) => (part as HTMLElement).dataset.tone)).toEqual(['game', 'game-2', 'other']);
  });

  it('a game whose size is not fully measured is said as “at least”, the rest as “at most”', () => {
    libraryGames.set([game('g-a', 'A', 10 * GB), game('g-b', 'B', 2 * GB, { sizeUnknown: true })]);
    storageInfo.set(info);
    renderDom(Installed);

    expect(legend()).toEqual([
      `A ${bytesSize(10 * GB)}`,
      `B ${words('common.meterAtLeast', 2 * GB)}`,
      `${msg('common.meterOther')} ${words('common.meterAtMost', 48 * GB)}`,
      `${msg('common.meterFree')} ${bytesSize(40 * GB)}`,
      msg('common.meterTotal', { total: bytesSize(100 * GB) }),
    ]);
  });

  it('a total the disk did not give is the meter’s unknown total, never “NaN%”', () => {
    libraryGames.set([game('g-a', 'A', 10 * GB)]);
    storageInfo.set({ ...info, totalBytes: 0, usedBytes: 0 });
    renderDom(Installed);

    const card = screen.getByText(msg('games.installedStorageTitle')).closest('.storage') as HTMLElement;
    expect(card.textContent).not.toMatch(/NaN|Infinity/);
    expect(card.textContent).toContain(msg('common.meterTotalUnknown'));
    expect(card.querySelector('.bar')?.getAttribute('data-state')).toBe('unknown');
    expect(screen.queryByText(msg('games.installedStorageUsage', { used: bytesSize(0), total: bytesSize(0) }))).toBeNull();
  });

  it('many small games are folded into one part named by the games’ own words', () => {
    libraryGames.set([game('g-big', 'Big', 80 * GB), ...Array.from({ length: 12 }, (_, i) => game(`g-${i}`, `Small ${i}`, 1024 ** 2))]);
    storageInfo.set({ ...info, freeBytes: 10 * GB, usedBytes: 90 * GB });
    renderDom(Installed);

    expect(legend()[1]).toBe(`${msg('installed.meterMoreGames', { count: 12 })} ${bytesSize(12 * 1024 ** 2)}`);
  });

  it('picking a game on the meter brings its row into view and focuses it, clearing a search that hid it', async () => {
    libraryGames.set([game('g-a', 'A', 10 * GB), game('g-b', 'B', 5 * GB)]);
    storageInfo.set(info);
    const { user } = renderDom(Installed);
    const scrolled = vi.fn();
    Element.prototype.scrollIntoView = scrolled;
    await user.type(screen.getByRole('textbox', { name: msg('games.installedSearchPlaceholder') }), 'A');
    expect(document.querySelector('[data-game-row="g-b"]')).toBeNull();

    await user.click(screen.getByRole('button', { name: `B: ${bytesSize(5 * GB)}` }));

    await vi.waitFor(() => expect(document.activeElement?.closest('[data-game-row]')?.getAttribute('data-game-row')).toBe('g-b'));
    expect(scrolled).toHaveBeenCalled();
  });

  it('says it is reading the disk until the first answer, not nothing', () => {
    renderDom(Installed);

    expect(screen.getByText(msg('installed.storageLoading'))).toBeTruthy();
  });

  it('“Manage storage” leads to the general settings', async () => {
    storageInfo.set(info);
    const { user } = renderDom(Installed);

    await user.click(screen.getByRole('button', { name: msg('games.installedManageStorage') }));

    expect(get(route)).toMatchObject({ name: 'settings', params: { tab: 'general' } });
  });

  it('a failed read is a danger notice with Retry, not the card', () => {
    storageFailed.set(true);
    renderDom(Installed);

    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    expect(notice.textContent).toContain(msg('games.installedStorageLoadFailed'));
    expect(within(notice).getByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(screen.queryByText(msg('games.installedStorageTitle'))).toBeNull();
  });
});

describe('Installed: the storage numbers stay true', () => {
  const reads = () => vi.mocked(getStorageInfo).mock.calls.length;
  const move = (stage: MoveJob['stage']) => ({ id: 'm1', scope: 'game', stage, gameId: 'g-a', title: 'A' }) as MoveJob;

  it('are read when the screen is entered', () => {
    renderDom(Installed);

    expect(reads()).toBe(1);
  });

  it('are read again when an installed game comes, goes, moves or is measured, not when a game merely changes', async () => {
    libraryGames.set([game('g-a', 'A', 10 * GB)]);
    renderDom(Installed);
    await tick();

    libraryGames.set([{ ...game('g-a', 'A', 10 * GB), lastPlayed: '2026-11-20T10:00:00Z' } as LibraryGame]);
    await tick();
    expect(reads()).toBe(1);

    libraryGames.set([game('g-a', 'A', 12 * GB)]);
    await tick();
    expect(reads()).toBe(2);

    libraryGames.set([game('g-a', 'A', 12 * GB, { installDir: 'C:\\Moved\\A' })]);
    await tick();
    expect(reads()).toBe(3);

    libraryGames.set([]);
    await tick();
    expect(reads()).toBe(4);
  });

  it('are read again when a move ends', async () => {
    renderDom(Installed);
    moves.set([move('copy')]);
    await tick();
    expect(reads()).toBe(1);

    moves.set([move('done')]);
    await tick();

    expect(reads()).toBe(2);
  });

  it('follow the library folder Go reads, not the games folder', async () => {
    renderDom(Installed);

    settings.set({ libraryPath: 'C:\\Games', gamesPath: 'E:\\Elsewhere' } as NonNullable<Parameters<typeof settings.set>[0]>);
    await tick();
    expect(reads()).toBe(1);

    settings.set({ libraryPath: 'D:\\Library', gamesPath: 'E:\\Elsewhere' } as NonNullable<Parameters<typeof settings.set>[0]>);
    await tick();
    expect(reads()).toBe(2);
  });

  it('are no longer followed once the screen is left', async () => {
    const view = renderDom(Installed);
    view.unmount();

    libraryGames.set([game('g-a', 'A', 10 * GB)]);
    moves.set([move('done')]);
    await tick();

    expect(reads()).toBe(1);
  });
});
