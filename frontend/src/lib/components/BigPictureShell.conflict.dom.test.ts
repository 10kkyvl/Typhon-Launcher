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
import BigPictureHarness from '../testing/BigPictureHarness.svelte';

const { play, status, resolve } = vi.hoisted(() => ({ play: vi.fn(), status: vi.fn(), resolve: vi.fn() }));

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { PlayGame: play, GetGames: vi.fn(async () => []) } }));
vi.mock('../../../bindings/typhon/internal/cloudsaves', () => ({ Service: { Status: status, ResolveConflict: resolve } }));
vi.mock('../bigpicture/components/HomePage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/LibraryPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/CatalogPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/GamePage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/DownloadsPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/SettingsPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/ProfilePage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));
vi.mock('../bigpicture/components/SocialPage.svelte', async () => ({ default: (await import('../testing/BigPicturePageStub.svelte')).default }));

const conflicted = (): CloudStatus => ({
  enabled: true,
  signedIn: true,
  usedBytes: 0,
  quotaBytes: 0,
  maxBytes: 0,
  error: '',
  games: [
    {
      gameId: 'game-1',
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
let pad: { id: string; index: number; mapping: string; connected: boolean; axes: number[]; buttons: { pressed: boolean; value: number }[] };

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.spyOn(Element.prototype, 'getClientRects').mockImplementation(() => [{}] as unknown as DOMRectList);
  pad = { id: 'Xbox Wireless Controller', index: 0, mapping: 'standard', connected: true, axes: [0, 0, 0, 0], buttons: Array.from({ length: 16 }, () => ({ pressed: false, value: 0 })) };
  Object.defineProperty(navigator, 'getGamepads', { value: () => [pad], configurable: true });
  libraryGames.set([{ id: 'game-1', title: 'Hades' } as LibraryGame]);
  status.mockResolvedValue(conflicted());
});

afterEach(() => {
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([]);
  toasts.set([]);
  app.remove();
  play.mockReset();
  status.mockReset();
  resolve.mockReset();
  vi.restoreAllMocks();
  Reflect.deleteProperty(navigator, 'getGamepads');
});

async function press(button: number) {
  pad.buttons[button] = { pressed: true, value: 1 };
  await new Promise((done) => requestAnimationFrame(done));
  await new Promise((done) => requestAnimationFrame(done));
  pad.buttons[button] = { pressed: false, value: 0 };
  await new Promise((done) => requestAnimationFrame(done));
  await new Promise((done) => requestAnimationFrame(done));
}

// The page's Play goes through the shell's session to the real launch; the binding refuses it.
async function launchToConflict(how: 'keyboard' | 'gamepad') {
  render(BigPictureHarness, { target: app });
  const playButton = await screen.findByRole('button', { name: 'Play' });
  await vi.waitFor(() => expect(document.activeElement).toBe(playButton));
  const keys = user();
  if (how === 'keyboard') await keys.keyboard('{Enter}');
  else await press(0);
  const dialog = await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') });
  await within(dialog).findByText(`${msg('cloudsaves.conflictDevice')}: Deck`);
  const answer = () => document.querySelector('[data-big-picture] output')?.textContent ?? '';
  return { keys, playButton, dialog, answer };
}

describe('a launch held back by a save conflict in Big Picture', () => {
  it('opens the conflict dialog over the shell; the page shows no error while it is open', async () => {
    play.mockRejectedValue(new Error('typhon:cloud.conflict_unresolved: diverged'));
    const { dialog, answer } = await launchToConflict('keyboard');

    expect((document.querySelector('[data-big-picture]') as HTMLElement).contains(dialog)).toBe(false);
    await vi.waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
    expect(answer()).toBe('');
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('keyboard: Escape gives the launch up, focus returns to Play, and the same game can be launched again', async () => {
    play.mockRejectedValue(new Error('typhon:cloud.conflict_unresolved: diverged'));
    const { keys, playButton, answer } = await launchToConflict('keyboard');

    await keys.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    await vi.waitFor(() => expect(answer()).toBe('false'));
    expect(document.activeElement).toBe(playButton);
    expect(get(toasts)).toEqual([]);

    await keys.keyboard('{Enter}');
    expect(await screen.findByRole('dialog', { name: msg('cloudsaves.conflictTitle') })).toBeTruthy();
    expect(play).toHaveBeenCalledTimes(2);
  });

  it('gamepad: B gives the launch up', async () => {
    play.mockRejectedValue(new Error('typhon:cloud.conflict_unresolved: diverged'));
    const { dialog, answer } = await launchToConflict('gamepad');
    await vi.waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));

    await press(1);

    expect(screen.queryByRole('dialog')).toBeNull();
    await vi.waitFor(() => expect(answer()).toBe('false'));
  });

  it('gamepad: the D-pad reaches a choice and A applies it; the game then starts and the session goes on', async () => {
    play.mockRejectedValueOnce(new Error('typhon:cloud.conflict_unresolved: diverged')).mockResolvedValueOnce(undefined);
    resolve.mockResolvedValueOnce(undefined);
    const { dialog, answer } = await launchToConflict('gamepad');
    await vi.waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
    const keepMine = within(dialog).getByRole('button', { name: msg('cloudsaves.conflictKeepMine') });

    for (let step = 0; step < 6 && document.activeElement !== keepMine; step += 1) await press(15);
    expect(document.activeElement).toBe(keepMine);
    await press(0);

    await vi.waitFor(() => expect(answer()).toBe('true'));
    expect(resolve).toHaveBeenCalledWith('game-1', true);
    expect(play).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('says that the saves are still syncing as a notice, with no dialog', async () => {
    play.mockRejectedValueOnce(new Error('typhon:cloud.syncing: downloading'));
    render(BigPictureHarness, { target: app });
    const playButton = await screen.findByRole('button', { name: 'Play' });
    await vi.waitFor(() => expect(document.activeElement).toBe(playButton));

    await user().keyboard('{Enter}');

    expect(await screen.findByText(msg('cloud.syncing'))).toBeTruthy();
    expect(get(toasts).map((item) => item.kind)).toEqual(['info']);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.querySelector('[data-big-picture] output')?.textContent).toBe('false');
  });
});
