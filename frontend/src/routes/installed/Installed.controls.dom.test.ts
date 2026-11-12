import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { playGame, type LibraryGame } from '../../lib/services/library';
import { getStorageInfo } from '../../lib/services/system';
import { libraryGames } from '../../lib/stores/library';
import { settings } from '../../lib/stores/settings';
import { storageFailed, storageInfo } from '../../lib/stores/storage';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  playGame: vi.fn(async () => {}),
}));
vi.mock('../../lib/services/system', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/system')>()),
  getStorageInfo: vi.fn(),
}));

const games = [
  { id: 'g-hades', title: 'Hades', lastPlayed: '2026-11-01T10:00:00Z' },
  { id: 'g-celeste', title: 'Celeste', lastPlayed: null },
].map((game, i) => ({ ...game, installDir: `C:\\Games\\${i}`, executable: 'game.exe', sizeBytes: 0, uninstalled: false, tags: [] }) as unknown as LibraryGame);

// The accessible name of the one button whose name holds `title`, other than the row's menu button.
function nameOf(title: string): string {
  let found = '';
  const menu = msg('games.installedMenuFor', { title });
  screen.getByRole('button', {
    name: (name) => {
      if (!name.includes(title) || name === menu) return false;
      found = name;
      return true;
    },
  });
  return found;
}

const times = (text: string, word: string) => text.split(word).length - 1;

beforeEach(() => {
  libraryGames.set(games);
});

afterEach(() => {
  libraryGames.set([]);
  installedView.set('list');
  document.body.replaceChildren();
});

describe('Installed: a row of the list view', () => {
  it('is named by its title once: the picture says nothing of its own', () => {
    installedView.set('list');
    renderDom(Installed);

    const name = nameOf('Celeste');
    expect(name.startsWith('Celeste')).toBe(true);
    expect(times(name, 'Celeste')).toBe(1);
  });
});

describe('Installed: the play button on a cover', () => {
  afterEach(() => vi.mocked(playGame).mockReset());

  it('is the base icon button, says which game it starts, and the keyboard presses it', async () => {
    installedView.set('grid');
    const { user } = renderDom(Installed);

    const plays = screen.getAllByRole('button', { name: msg('games.play') });
    expect(plays.map((button) => button.classList.contains('icon-btn'))).toEqual([true, true]);
    const celeste = screen.getByRole('button', { name: msg('games.play'), description: 'Celeste' });
    celeste.focus();
    await user.keyboard('{Enter}');

    expect(playGame).toHaveBeenCalledWith('g-celeste');
  });
});

describe('Installed: retrying the disk usage', () => {
  afterEach(() => {
    settings.set(null);
    storageFailed.set(false);
    storageInfo.set(null);
    vi.mocked(getStorageInfo).mockReset();
  });

  it('keeps the danger notice and the focus on its busy Retry until the answer', async () => {
    let answer!: (info: Awaited<ReturnType<typeof getStorageInfo>>) => void;
    vi.mocked(getStorageInfo).mockReturnValue(new Promise((resolve) => (answer = resolve)));
    settings.set({ libraryPath: 'C:\\Games' } as NonNullable<Parameters<typeof settings.set>[0]>);
    storageFailed.set(true);
    const { user } = renderDom(Installed);
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    expect(notice.textContent).toContain(msg('games.installedStorageLoadFailed'));
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    answer({ usedBytes: 50, totalBytes: 100, freeBytes: 50 } as Awaited<ReturnType<typeof getStorageInfo>>);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(screen.getByText(msg('games.installedStorageTitle'))).toBeTruthy();
  });
});

describe('Installed: no library folder yet', () => {
  it('the storage card says so with the base empty state and offers to choose one', () => {
    renderDom(Installed);

    const empty = screen.getByText(msg('games.installedLibraryNotSetTitle')).closest('.empty.inline');
    expect(empty).not.toBeNull();
    expect(within(empty as HTMLElement).getByRole('button', { name: msg('games.installedChooseFolderButton') })).toBeTruthy();
  });
});
