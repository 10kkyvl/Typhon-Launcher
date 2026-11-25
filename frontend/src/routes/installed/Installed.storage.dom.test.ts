import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import type { StorageInfo } from '../../lib/services/system';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
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
  vi.clearAllMocks();
  document.body.replaceChildren();
});

const legend = () => [...document.querySelectorAll('.storage-legend li')].map((item) => item.textContent?.replace(/\s+/g, ' ').trim());

describe('Installed: the storage card', () => {
  it('says what the games on the library’s disk take, the rest and the free space', () => {
    libraryGames.set([game('g-a', 'A', 10 * GB), game('g-b', 'B', 5 * GB), game('g-d', 'D', 7 * GB, { installDir: 'D:\\Other\\D' })]);
    storageInfo.set(info);
    renderDom(Installed);

    expect(screen.getByText(msg('games.installedStorageTitle'))).toBeTruthy();
    expect(screen.getByText(msg('games.installedStorageUsage', { used: bytesSize(60 * GB), total: bytesSize(100 * GB) }))).toBeTruthy();
    expect(legend()).toEqual([
      `${msg('games.installedLegendGames')} ${bytesSize(15 * GB)}`,
      `${msg('games.installedLegendOther')} ${bytesSize(45 * GB)}`,
      `${msg('games.installedLegendFree')} ${bytesSize(40 * GB)}`,
    ]);
  });

  it('with a game whose size is not known, says only what is used and free', () => {
    libraryGames.set([game('g-a', 'A', 10 * GB), game('g-b', 'B', 0, { sizeUnknown: true })]);
    storageInfo.set(info);
    renderDom(Installed);

    expect(legend()).toEqual([`${msg('games.installedLegendUsed')} ${bytesSize(60 * GB)}`, `${msg('games.installedLegendFree')} ${bytesSize(40 * GB)}`]);
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
