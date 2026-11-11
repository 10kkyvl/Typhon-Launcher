import { get } from 'svelte/store';
import { within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { CloudGameStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import { applyUpdate } from '../services/selfupdate';
import { closeCloudConflict, cloudConflictTarget, cloudStatus } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { selfUpdateStatus } from '../stores/selfupdate';
import { authState } from '../stores/user';
import { renderDom } from '../testing/dom';
import SidebarStatus from './SidebarStatus.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/selfupdate')>()),
  applyUpdate: vi.fn(async () => {}),
}));

const game = (gameId: string, state: CloudGameStatus['state']): CloudGameStatus => ({
  gameId,
  state,
  code: '',
  message: '',
  at: '',
  syncedAt: '',
});

beforeEach(() => {
  vi.clearAllMocks();
  authState.set('authenticated');
});

afterEach(() => {
  cloudStatus.set(null);
  libraryGames.set([]);
  closeCloudConflict();
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  authState.set('bootstrapping');
});

describe('SidebarStatus in a DOM', () => {
  it('the conflict state opens the conflict of the first conflicted game the library has', async () => {
    libraryGames.set([
      { id: 'synced', title: 'Hollow' },
      { id: 'first', title: 'Celeste' },
      { id: 'second', title: 'Tunic' },
    ] as LibraryGame[]);
    cloudStatus.set({
      enabled: true,
      signedIn: true,
      usedBytes: 0,
      quotaBytes: 0,
      maxBytes: 0,
      error: '',
      games: [game('removed', 'conflict'), game('synced', 'synced'), game('first', 'conflict'), game('second', 'conflict')],
    });
    const { getByRole, user } = renderDom(SidebarStatus);

    await user.click(getByRole('button', { name: msg('ui.statusCloudConflicts', { count: 2 }) }));

    expect(get(cloudConflictTarget)).toEqual({ gameId: 'first', title: 'Celeste' });
  });

  it('a ready update asks for the restart first and applies it only on confirmation', async () => {
    selfUpdateStatus.set({ state: 'ready', currentVersion: '1.4.1', availableVersion: '1.5.0' });
    const { getByRole, queryByRole, user } = renderDom(SidebarStatus);
    expect(queryByRole('dialog')).toBeNull();

    await user.click(getByRole('button', { name: msg('ui.restartAndUpdate') }));

    const dialog = getByRole('dialog', { name: msg('ui.restartLauncherTitle') });
    expect(vi.mocked(applyUpdate)).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: msg('ui.restartAndUpdate') }));
    expect(vi.mocked(applyUpdate)).toHaveBeenCalledTimes(1);
    expect(queryByRole('dialog')).toBeNull();
  });

  it('cancelling the restart confirmation applies nothing', async () => {
    selfUpdateStatus.set({ state: 'ready', currentVersion: '1.4.1', availableVersion: '1.5.0' });
    const { getByRole, queryByRole, user } = renderDom(SidebarStatus);
    await user.click(getByRole('button', { name: msg('ui.restartAndUpdate') }));

    await user.click(within(getByRole('dialog')).getByRole('button', { name: msg('common.cancel') }));

    expect(queryByRole('dialog')).toBeNull();
    expect(vi.mocked(applyUpdate)).not.toHaveBeenCalled();
  });
});
