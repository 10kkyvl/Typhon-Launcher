import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { CloudStatus } from '../services/cloudsaves';
import type { Installation } from '../services/install';
import type { LibraryGame } from '../services/library';
import { closeCloudConflict, cloudStatus } from '../stores/cloudsaves';
import { installations } from '../stores/install';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import CloudConflictModal from './CloudConflictModal.svelte';
import InstallModal from './InstallModal.svelte';

const { play, status, resolve } = vi.hoisted(() => ({ play: vi.fn(), status: vi.fn(), resolve: vi.fn() }));

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { PlayGame: play, GetGames: vi.fn(async () => []) } }));
vi.mock('../../../bindings/typhon/internal/cloudsaves', () => ({ Service: { Status: status, ResolveConflict: resolve } }));
vi.mock('../services/emulators', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/emulators')>()),
  listPlatforms: vi.fn(async () => []),
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

// An image installed for an emulator: the dialog ends on "Play".
const installed = { id: 'i1', downloadId: 'd1', gameId: 'g1', name: 'Hades', type: 'console_image', status: 'completed', platform: 'ps2', destination: '/games/hades' } as Installation;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  installations.set([installed]);
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
  status.mockResolvedValue(conflicted());
});

afterEach(() => {
  closeCloudConflict();
  cloudStatus.set(null);
  installations.set([]);
  libraryGames.set([]);
  toasts.set([]);
  play.mockReset();
  status.mockReset();
  resolve.mockReset();
  document.body.replaceChildren();
});

function openDone() {
  const view = render(InstallModal, { target: app, props: { open: true, downloadId: 'd1' } });
  render(CloudConflictModal, { target: app });
  const dialog = screen.getByRole('dialog', { name: msg('modals.installTitle') });
  return { view, dialog, playButton: within(dialog).getByRole('button', { name: msg('games.play') }) };
}

describe('InstallModal: "Play" after the installation', () => {
  it.each(['library.launch_failed', 'library.executable_missing', 'emulators.not_configured'])(
    'shows a failed launch in the launch error’s own words (%s)',
    async (code) => {
      // An emulator code with no known platform for the game is shown as it is, not asked about.
      play.mockRejectedValue(new Error(`typhon:${code}: detail`));
      const { playButton } = openDone();

      await user().click(playButton);

      await vi.waitFor(() => expect(get(toasts).map(({ kind, message }) => [kind, message])).toEqual([['danger', msg(code as 'library.launch_failed')]]));
    },
  );

  it('stays open after "Later" in the conflict dialog over it, with the focus back on its busy-free Play', async () => {
    play.mockRejectedValue(new Error('typhon:cloud.conflict_unresolved: diverged'));
    const { dialog, playButton } = openDone();
    const keys = user();
    playButton.focus();

    await keys.keyboard('{Enter}');

    const conflict = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    expect(playButton.getAttribute('aria-busy')).toBe('true');
    expect(playButton).toHaveProperty('disabled', false);
    await within(conflict).findByText(`${msg('cloudsaves.conflictDevice')}: Deck`);

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('dialog', { name: msg('cloudsaves.conflictTitle') })).toBeNull();
    expect(dialog.isConnected).toBe(true);
    await vi.waitFor(() => expect(playButton.getAttribute('aria-busy')).toBeNull());
    expect(document.activeElement).toBe(playButton);
    expect(get(toasts)).toEqual([]);
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('closes once the game started after the conflict was resolved', async () => {
    play.mockRejectedValueOnce(new Error('typhon:cloud.conflict_unresolved: diverged')).mockResolvedValueOnce(undefined);
    resolve.mockResolvedValueOnce(undefined);
    const { playButton } = openDone();
    const keys = user();

    await keys.click(playButton);
    const conflict = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
    await within(conflict).findByText(`${msg('cloudsaves.conflictDevice')}: Deck`);
    await keys.click(within(conflict).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog', { name: msg('modals.installTitle') })).toBeNull());
    expect(play).toHaveBeenCalledTimes(2);
    expect(get(toasts).map((item) => item.kind)).toEqual(['success']);
  });
});
