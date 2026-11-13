import { beforeEach, describe, expect, it, vi } from 'vitest';

const { play, playProfile, games } = vi.hoisted(() => ({ play: vi.fn(), playProfile: vi.fn(), games: vi.fn() }));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { PlayGame: play, PlayGameProfile: playProfile, GetGames: games } }));
vi.mock('./backend', () => ({ inWails: true }));

import { get } from 'svelte/store';
import { msg } from '../i18n';
import { launchGame, playGameProfile, type LibraryGame } from './library';
import { closeCloudConflict, cloudConflictTarget, finishCloudConflict, openCloudConflict } from '../stores/cloudsaves';
import { emulatorPrompt } from '../stores/emulatorPrompt';
import { libraryGames } from '../stores/library';
import { toasts } from '../stores/toasts';

const fails = (code: string) => new Error(`typhon:${code}: detail`);

// The two entry points of an in-window launch, with the binding each one calls.
const entries = [
  { name: 'launchGame', binding: play, launch: () => launchGame('g1'), args: ['g1'] },
  { name: 'playGameProfile', binding: playProfile, launch: () => playGameProfile('g1', 'p1'), args: ['g1', 'p1'] },
] as const;

// What the launch has answered so far: still waiting, its result, or what it threw.
async function outcome(launch: Promise<boolean>): Promise<boolean | 'waiting' | { thrown: unknown }> {
  const caught = launch.catch((err: unknown) => ({ thrown: err }));
  return await Promise.race([caught, new Promise<'waiting'>((resolve) => setTimeout(() => resolve('waiting'), 0))]);
}

async function dialogFor(gameId: string) {
  await vi.waitFor(() => expect(get(cloudConflictTarget)?.gameId).toBe(gameId));
  return get(cloudConflictTarget)!;
}

const toastKinds = () => get(toasts).map((item) => item.kind);

beforeEach(() => {
  play.mockReset();
  playProfile.mockReset();
  games.mockReset();
  closeCloudConflict();
  emulatorPrompt.set(null);
  toasts.set([]);
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
  games.mockResolvedValue([{ id: 'g1', emulation: { platform: 'ps2' } }]);
});

describe.each(entries)('$name: a launch held back by a save conflict', ({ binding, launch, args }) => {
  it('opens the conflict dialog for the game, waits on it, and launches the same way again once after the choice', async () => {
    binding.mockRejectedValueOnce(fails('cloud.conflict_unresolved')).mockResolvedValueOnce(undefined);
    const launched = launch();

    const target = await dialogFor('g1');
    expect(target.title).toBe('Hades');
    expect(await outcome(launched)).toBe('waiting');
    expect(binding).toHaveBeenCalledTimes(1);

    finishCloudConflict(target);

    expect(await outcome(launched)).toBe(true);
    expect(binding).toHaveBeenCalledTimes(2);
    expect(binding).toHaveBeenLastCalledWith(...args);
    expect(toastKinds()).toEqual([]);
  });

  it('is given up when the player leaves the dialog without a choice: nothing is launched and nothing is said', async () => {
    binding.mockRejectedValueOnce(fails('cloud.conflict_unresolved'));
    const launched = launch();
    await dialogFor('g1');

    closeCloudConflict();

    expect(await outcome(launched)).toBe(false);
    expect(binding).toHaveBeenCalledTimes(1);
    expect(toastKinds()).toEqual([]);
  });

  it('shows a second conflict as an error instead of asking again', async () => {
    const second = fails('cloud.conflict_unresolved');
    binding.mockRejectedValueOnce(fails('cloud.conflict_unresolved')).mockRejectedValueOnce(second);
    const launched = launch();
    finishCloudConflict(await dialogFor('g1'));

    expect(await outcome(launched)).toEqual({ thrown: second });
    expect(get(cloudConflictTarget)).toBeNull();
    expect(binding).toHaveBeenCalledTimes(2);
  });

  it('shows the conflict as an error when the game is not in the library store', async () => {
    const failure = fails('cloud.conflict_unresolved');
    libraryGames.set([]);
    binding.mockRejectedValueOnce(failure);

    expect(await outcome(launch())).toEqual({ thrown: failure });
    expect(get(cloudConflictTarget)).toBeNull();
    expect(binding).toHaveBeenCalledTimes(1);
  });

  it('joins the dialog an event already opened for the game, and still launches after the choice', async () => {
    openCloudConflict('g1');
    const opened = get(cloudConflictTarget)!;
    binding.mockRejectedValueOnce(fails('cloud.conflict_unresolved')).mockResolvedValueOnce(undefined);
    const launched = launch();
    await Promise.resolve();

    expect(get(cloudConflictTarget)).toBe(opened);
    finishCloudConflict(opened);
    expect(await outcome(launched)).toBe(true);
    expect(binding).toHaveBeenCalledTimes(2);
  });

  it('is given up when the dialog goes to another game before the player chose', async () => {
    binding.mockRejectedValueOnce(fails('cloud.conflict_unresolved'));
    const launched = launch();
    await dialogFor('g1');

    openCloudConflict('g2');

    expect(await outcome(launched)).toBe(false);
    expect(get(cloudConflictTarget)?.gameId).toBe('g2');
    expect(binding).toHaveBeenCalledTimes(1);
  });

  it('says the saves are still syncing as a notice, not a failure, and gives the launch up', async () => {
    binding.mockRejectedValueOnce(fails('cloud.syncing'));

    expect(await outcome(launch())).toBe(false);
    expect(get(toasts).map(({ kind, message }) => [kind, message])).toEqual([['info', msg('cloud.syncing')]]);
    expect(get(cloudConflictTarget)).toBeNull();
    expect(binding).toHaveBeenCalledTimes(1);
  });
});

describe('the launch made again after the conflict was resolved', () => {
  it('launchGame: goes through the usual recovery when it fails with something else', async () => {
    play.mockRejectedValueOnce(fails('cloud.conflict_unresolved')).mockRejectedValueOnce(fails('emulators.not_configured')).mockResolvedValueOnce(undefined);
    const launched = launchGame('g1');
    finishCloudConflict(await dialogFor('g1'));

    await vi.waitFor(() => expect(get(emulatorPrompt)?.platform).toBe('ps2'));
    get(emulatorPrompt)!.settle(true);

    expect(await outcome(launched)).toBe(true);
    expect(play).toHaveBeenCalledTimes(3);
  });

  it('playGameProfile: shows any other failure as it is', async () => {
    const failure = fails('library.launch_failed');
    playProfile.mockRejectedValueOnce(fails('cloud.conflict_unresolved')).mockRejectedValueOnce(failure);
    const launched = playGameProfile('g1', 'p1');
    finishCloudConflict(await dialogFor('g1'));

    expect(await outcome(launched)).toEqual({ thrown: failure });
  });
});

describe('a profile launch and the emulator questions', () => {
  it('shows an emulator error as it is, as before: the emulator questions belong to the game launch', async () => {
    const failure = fails('emulators.not_configured');
    playProfile.mockRejectedValueOnce(failure);

    expect(await outcome(playGameProfile('g1', 'p1'))).toEqual({ thrown: failure });
    expect(get(emulatorPrompt)).toBeNull();
  });

  it('reports a cancelled profile launch as given up, as before', async () => {
    playProfile.mockRejectedValueOnce(fails('library.launch_cancelled'));
    expect(await outcome(playGameProfile('g1', 'p1'))).toBe(false);
  });
});
