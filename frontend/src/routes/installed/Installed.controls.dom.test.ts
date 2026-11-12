import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { playGame, type LibraryGame } from '../../lib/services/library';
import { getStorageInfo } from '../../lib/services/system';
import type { Update } from '../../lib/services/updates';
import { libraryGames } from '../../lib/stores/library';
import { settings } from '../../lib/stores/settings';
import { storageFailed, storageInfo } from '../../lib/stores/storage';
import { installedView } from '../../lib/stores/ui';
import { updates } from '../../lib/stores/updates';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
// Only the add-game dialog needs the desktop app: its test turns this on before pressing "Add game".
const backend = vi.hoisted(() => ({ inWails: false }));
vi.mock('../../lib/services/backend', () => backend);
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  playGame: vi.fn(async () => {}),
  addGame: vi.fn(),
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

describe('Installed: an update waiting for a game', () => {
  afterEach(() => updates.set([]));

  it('is said with the base status badge, in the row and on the cover', () => {
    updates.set([{ gameId: 'g-celeste', availability: { available: true, kind: 'update' } } as unknown as Update]);
    installedView.set('list');
    const list = renderDom(Installed);
    const inRow = screen.getByText(msg('games.updateKindUpdate'));
    expect(inRow.closest('.badge.warning')).not.toBeNull();
    list.unmount();

    installedView.set('grid');
    renderDom(Installed);
    expect(screen.getByText(msg('games.updateKindUpdate')).closest('.badge.warning')).not.toBeNull();
    expect(screen.getByRole('button', { name: `Celeste — ${msg('games.updateKindUpdate')}` })).toBeTruthy();
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
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('games.installedManageStorage') })));
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

describe('Installed: adding a game by hand', () => {
  afterEach(() => {
    backend.inWails = false;
  });

  async function openDialog() {
    const view = renderDom(Installed);
    backend.inWails = true;
    await view.user.click(screen.getByRole('button', { name: msg('games.installedAddGameButton') }));
    return { view, dialog: await screen.findByRole('dialog') };
  }

  it('names the executable field by its label alone, Browse is a button beside it', async () => {
    const { dialog } = await openDialog();

    const field = within(dialog).getByRole('textbox', { name: msg('games.executableLabel') });
    const browse = within(dialog).getByRole('button', { name: msg('games.installedBrowseButton') });
    expect(browse.closest('label')).toBeNull();
    expect(field.closest('label')).toBeNull();
  });

});
