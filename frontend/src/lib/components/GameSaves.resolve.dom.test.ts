import { render, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { cloudResolveConflict, type CloudGameStatus, type CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import type { Settings } from '../services/settings';
import { closeCloudConflict, cloudStatus } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { saveBackupsEnabled } from '../stores/savebackup';
import { settings } from '../stores/settings';
import { toasts } from '../stores/toasts';
import { authState } from '../stores/user';
import { user } from '../testing/dom';
import CloudConflictModal from './CloudConflictModal.svelte';
import GameSaves from './GameSaves.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/cloudsaves', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/cloudsaves')>()),
  cloudResolveConflict: vi.fn(),
}));

const conflict = { localDigest: 'a', localUpdatedAt: '2026-11-01T10:00:00Z', localSize: 10, remoteDigest: 'b', remoteUpdatedAt: '2026-11-02T10:00:00Z', remoteSize: 20, remoteDevice: 'Deck' };
const game = (patch: Partial<CloudGameStatus>): CloudGameStatus => ({ gameId: 'g1', state: 'synced', code: '', message: '', at: '', syncedAt: '', ...patch });
const status = (g: CloudGameStatus): CloudStatus => ({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games: [g] });

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  libraryGames.set([{ id: 'g1', title: 'Hades', canonicalGameId: 'igdb:1' } as LibraryGame]);
  settings.set({ cloudSaves: true } as Settings);
  authState.set('authenticated');
  saveBackupsEnabled.set(false);
  cloudStatus.set(status(game({ state: 'conflict', conflict })));
  vi.mocked(cloudResolveConflict).mockResolvedValue();
});

afterEach(() => {
  closeCloudConflict();
  libraryGames.set([]);
  settings.set(null);
  authState.set('bootstrapping');
  cloudStatus.set(null);
  toasts.set([]);
  vi.mocked(cloudResolveConflict).mockReset();
  document.body.replaceChildren();
});

describe('GameSaves: the conflict resolved from its Resolve', () => {
  it('moves focus on to Sync now once the choice is applied and Resolve goes', async () => {
    render(GameSaves, { target: app, props: { gameId: 'g1' } });
    render(CloudConflictModal, { target: app });
    const keys = user();
    screen.getByRole('button', { name: msg('cloudsaves.resolve') }).focus();
    await keys.keyboard('{Enter}');
    const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    // The status loaded again after the choice: the game is no longer in conflict.
    cloudStatus.set(status(game({ state: 'synced', syncedAt: '2026-11-03T10:00:00Z' })));
    await tick();

    expect(screen.queryByRole('button', { name: msg('cloudsaves.resolve') })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('cloudsaves.syncNow') })));
  });

  it('leaves focus where the player put it when the conflict goes meanwhile', async () => {
    render(GameSaves, { target: app, props: { gameId: 'g1' } });
    const field = document.createElement('input');
    app.append(field);
    field.focus();

    cloudStatus.set(status(game({ state: 'synced' })));
    await tick();
    await tick();

    expect(document.activeElement).toBe(field);
  });
});
