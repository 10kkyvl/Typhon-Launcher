import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import { closeCloudConflict, cloudStatus } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import CloudConflictModal from './CloudConflictModal.svelte';
import RandomGameModal from './RandomGameModal.svelte';

const { play, status, resolve } = vi.hoisted(() => ({ play: vi.fn(), status: vi.fn(), resolve: vi.fn() }));

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { PlayGame: play, GetGames: vi.fn(async () => []) } }));
vi.mock('../../../bindings/typhon/internal/cloudsaves', () => ({ Service: { Status: status, ResolveConflict: resolve } }));
// The metadata service tells the backend the locale as soon as it loads (in Wails).
vi.mock('../../../bindings/typhon/internal/metadata', () => ({ Service: { SetLanguage: vi.fn(async () => {}) } }));
vi.mock('../game/randomPick', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../game/randomPick')>()),
  reducedMotion: () => true,
}));

const conflicted = (): CloudStatus => ({
  enabled: true,
  signedIn: true,
  usedBytes: 0,
  quotaBytes: 0,
  maxBytes: 0,
  error: '',
  games: [
    {
      gameId: 'g1',
      state: 'conflict',
      code: '',
      message: '',
      at: '',
      syncedAt: '',
      conflict: { localDigest: 'a', localUpdatedAt: '2026-11-01T10:00:00Z', localSize: 10, remoteDigest: 'b', remoteUpdatedAt: '2026-11-02T10:00:00Z', remoteSize: 20, remoteDevice: 'Deck' },
    },
  ],
});

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', uninstalled: false, installDir: '/games/hades', executable: 'Hades.exe' } as LibraryGame]);
  status.mockResolvedValue(conflicted());
});

afterEach(() => {
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([]);
  toasts.set([]);
  play.mockReset();
  status.mockReset();
  resolve.mockReset();
  document.body.replaceChildren();
});

// Spins to the one game in the library and answers its "Play" button.
async function pickedGame() {
  const onclose = vi.fn();
  render(RandomGameModal, { target: app, props: { onclose } });
  render(CloudConflictModal, { target: app });
  const dialog = screen.getByRole('dialog', { name: msg('games.randomTitle') });
  const keys = user();
  await keys.click(within(dialog).getByRole('button', { name: msg('games.randomSpin') }));
  const playButton = await within(dialog).findByRole('button', { name: msg('games.randomPlay') });
  return { onclose, dialog, keys, playButton };
}

describe('RandomGameModal: a launch held back by a save conflict', () => {
  it('stays open after "Later" in the conflict dialog over it, with the focus back on Play', async () => {
    play.mockRejectedValue(new Error('typhon:cloud.conflict_unresolved: diverged'));
    const { onclose, dialog, keys, playButton } = await pickedGame();
    playButton.focus();

    await keys.keyboard('{Enter}');

    const conflict = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    expect(playButton.getAttribute('aria-busy')).toBe('true');
    expect(playButton).toHaveProperty('disabled', false);
    await within(conflict).findByText(`${msg('cloudsaves.conflictDevice')}: Deck`);

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('dialog', { name: msg('cloudsaves.conflictTitle') })).toBeNull();
    await vi.waitFor(() => expect(playButton.getAttribute('aria-busy')).toBeNull());
    expect(onclose).not.toHaveBeenCalled();
    expect(dialog.isConnected).toBe(true);
    expect(document.activeElement).toBe(playButton);
    expect(get(toasts)).toEqual([]);
  });

  it('closes once the game started after the conflict was resolved', async () => {
    play.mockRejectedValueOnce(new Error('typhon:cloud.conflict_unresolved: diverged')).mockResolvedValueOnce(undefined);
    resolve.mockResolvedValueOnce(undefined);
    const { onclose, keys, playButton } = await pickedGame();

    await keys.click(playButton);
    const conflict = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    await within(conflict).findByText(`${msg('cloudsaves.conflictDevice')}: Deck`);
    await keys.click(within(conflict).getByRole('button', { name: msg('cloudsaves.conflictTakeRemote') }));

    await vi.waitFor(() => expect(onclose).toHaveBeenCalledTimes(1));
    expect(play).toHaveBeenCalledTimes(2);
  });

  it('stays open, with no error, when the saves are still syncing; the notice says so', async () => {
    play.mockRejectedValueOnce(new Error('typhon:cloud.syncing: downloading'));
    const { onclose, keys, playButton } = await pickedGame();

    await keys.click(playButton);

    await vi.waitFor(() => expect(get(toasts).map(({ kind, message }) => [kind, message])).toEqual([['info', msg('cloud.syncing')]]));
    expect(onclose).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog', { name: msg('cloudsaves.conflictTitle') })).toBeNull();
  });
});
