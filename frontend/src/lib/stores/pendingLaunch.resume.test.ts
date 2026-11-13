import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// A launch started outside the window and left pending in Go, taken by the page
// and resumed through the in-window launch path with its real recovery.
const { play, playProfile, games, takePending, handlers } = vi.hoisted(() => ({
  play: vi.fn(),
  playProfile: vi.fn(),
  games: vi.fn(),
  takePending: vi.fn(),
  handlers: [] as Array<() => void>,
}));

vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Events: {
    On: vi.fn((_name: string, handler: () => void) => {
      handlers.push(handler);
      return () => {};
    }),
  },
}));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { PlayGame: play, PlayGameProfile: playProfile, GetGames: games } }));
vi.mock('../../../bindings/typhon/pendinglaunchservice', () => ({ TakePending: takePending }));
vi.mock('../services/backend', () => ({ inWails: true }));

import { get } from 'svelte/store';
import { msg } from '../i18n';
import type { LibraryGame } from '../services/library';
import { closeCloudConflict, cloudConflictTarget, finishCloudConflict } from './cloudsaves';
import { initPendingLaunch, setWindowReady } from './pendingLaunch';
import { emulatorPrompt } from './emulatorPrompt';
import { libraryGames, libraryLoaded } from './library';
import { toasts } from './toasts';

const nothing = { gameId: '', platform: '', code: '', profileId: '', profileLaunch: false };
const fails = (code: string) => new Error(`typhon:${code}: detail`);
const flush = () => new Promise((resolve) => setTimeout(resolve, 0));
const toastList = () => get(toasts).map(({ kind, message }) => [kind, message]);

let dispose: () => void = () => {};

// The two gates of the window: past the sign-in, legal and consent screens,
// and the library list loaded.
function passSignIn() {
  setWindowReady(true);
}
function loadLibrary() {
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
  libraryLoaded.set(true);
}

async function dialogFor(gameId: string) {
  await vi.waitFor(() => expect(get(cloudConflictTarget)?.gameId).toBe(gameId));
  return get(cloudConflictTarget)!;
}

beforeEach(() => {
  handlers.length = 0;
  play.mockReset();
  playProfile.mockReset();
  games.mockReset().mockResolvedValue([{ id: 'g1', emulation: { platform: 'ps2' } }]);
  takePending.mockReset().mockResolvedValue(nothing);
  closeCloudConflict();
  emulatorPrompt.set(null);
  toasts.set([]);
  setWindowReady(false);
  libraryGames.set([]);
  libraryLoaded.set(false);
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  dispose();
  get(emulatorPrompt)?.settle(false);
});

// How each outside entry point launched the game, and the in-window launch that repeats it.
const entries = [
  { name: 'tray', held: { profileId: '', profileLaunch: false }, binding: play, args: ['g1'] },
  { name: 'shortcut', held: { profileId: '', profileLaunch: true }, binding: playProfile, args: ['g1', ''] },
  { name: 'shortcut with a profile', held: { profileId: 'ab12cd34', profileLaunch: true }, binding: playProfile, args: ['g1', 'ab12cd34'] },
] as const;

const heldBack = (code: string, held: { profileId: string; profileLaunch: boolean }, gameId = 'g1') => ({ gameId, platform: '', code, ...held });

describe.each(entries)('$name: a launch the save conflict held back outside the window', ({ held, binding, args }) => {
  it('waits for the window, makes the same launch, and launches once more after the choice', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', held));
    binding.mockRejectedValueOnce(fails('cloud.conflict_unresolved')).mockResolvedValueOnce(undefined);

    dispose = initPendingLaunch();
    await flush();
    passSignIn();
    await flush();
    expect(binding).not.toHaveBeenCalled();
    expect(play).not.toHaveBeenCalled();
    expect(playProfile).not.toHaveBeenCalled();

    loadLibrary();
    const target = await dialogFor('g1');
    expect(binding).toHaveBeenCalledTimes(1);
    expect(binding).toHaveBeenLastCalledWith(...args);

    finishCloudConflict(target);
    await vi.waitFor(() => expect(binding).toHaveBeenCalledTimes(2));
    expect(binding).toHaveBeenLastCalledWith(...args);
    await flush();
    expect(toastList()).toEqual([]);
  });

  it('starts nothing and says nothing when the player leaves the dialog', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', held));
    binding.mockRejectedValueOnce(fails('cloud.conflict_unresolved'));
    loadLibrary();

    dispose = initPendingLaunch();
    passSignIn();
    await dialogFor('g1');
    closeCloudConflict();
    await flush();

    expect(binding).toHaveBeenCalledTimes(1);
    expect(toastList()).toEqual([]);
  });

  it('shows the syncing notice the resumed launch answers', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.syncing', held));
    binding.mockRejectedValueOnce(fails('cloud.syncing'));

    dispose = initPendingLaunch();
    passSignIn();
    loadLibrary();

    await vi.waitFor(() => expect(toastList()).toEqual([['info', msg('cloud.syncing')]]));
    expect(binding).toHaveBeenCalledTimes(1);
    expect(binding).toHaveBeenLastCalledWith(...args);
  });

  it('starts the game when the saves finished syncing before the window resumed it', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.syncing', held));
    binding.mockResolvedValueOnce(undefined);

    dispose = initPendingLaunch();
    passSignIn();
    loadLibrary();

    await vi.waitFor(() => expect(binding).toHaveBeenCalledTimes(1));
    expect(binding).toHaveBeenLastCalledWith(...args);
    await flush();
    expect(toastList()).toEqual([]);
    expect(get(cloudConflictTarget)).toBeNull();
  });

  it('drops the resume silently when the game is already running or starting', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', held));
    binding.mockRejectedValueOnce(fails('library.already_running'));

    dispose = initPendingLaunch();
    passSignIn();
    loadLibrary();

    await vi.waitFor(() => expect(binding).toHaveBeenCalledTimes(1));
    await flush();
    expect(toastList()).toEqual([]);
    expect(get(cloudConflictTarget)).toBeNull();
  });

  it('shows any other answer of the resumed launch as a launch error', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', held));
    binding.mockRejectedValueOnce(fails('library.game_not_found'));

    dispose = initPendingLaunch();
    passSignIn();
    loadLibrary();

    await vi.waitFor(() => expect(toastList()).toEqual([['danger', msg('library.game_not_found')]]));
    expect(get(cloudConflictTarget)).toBeNull();
  });
});

describe('the gates a held-back launch waits for', () => {
  it('waits for the sign-in screens when the library is already loaded', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', entries[0].held));
    play.mockResolvedValue(undefined);
    loadLibrary();

    dispose = initPendingLaunch();
    await flush();
    expect(play).not.toHaveBeenCalled();

    passSignIn();
    await vi.waitFor(() => expect(play).toHaveBeenCalledTimes(1));
  });

  it('resumes once when the event and the poll at mount both look for one pending launch', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', entries[0].held));
    play.mockResolvedValue(undefined);

    dispose = initPendingLaunch();
    handlers[0]();
    await flush();
    passSignIn();
    loadLibrary();
    await vi.waitFor(() => expect(play).toHaveBeenCalledTimes(1));
    handlers[0]();
    await flush();

    expect(takePending).toHaveBeenCalledTimes(3);
    expect(play).toHaveBeenCalledTimes(1);
    expect(playProfile).not.toHaveBeenCalled();
  });

  it('resumes only the newer of two launches held back while the window was not ready', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', entries[0].held));
    playProfile.mockResolvedValue(undefined);

    dispose = initPendingLaunch();
    await flush();
    takePending.mockResolvedValueOnce(heldBack('cloud.syncing', entries[2].held, 'g2'));
    handlers[0]();
    await flush();
    passSignIn();
    loadLibrary();

    await vi.waitFor(() => expect(playProfile).toHaveBeenCalledWith('g2', 'ab12cd34'));
    await flush();
    expect(playProfile).toHaveBeenCalledTimes(1);
    expect(play).not.toHaveBeenCalled();
  });

  it('drops a held-back launch still waiting when a newer emulator prompt arrives', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', entries[2].held, 'g2'));

    dispose = initPendingLaunch();
    await flush();
    takePending.mockResolvedValueOnce({ gameId: 'g1', platform: 'ps2', code: 'emulators.not_configured', profileId: '', profileLaunch: false });
    handlers[0]();
    await vi.waitFor(() => expect(get(emulatorPrompt)?.platform).toBe('ps2'));
    passSignIn();
    loadLibrary();
    await flush();

    expect(playProfile).not.toHaveBeenCalled();
    expect(play).not.toHaveBeenCalled();
  });

  it('keeps nothing once the page is gone', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.conflict_unresolved', entries[0].held));
    play.mockResolvedValue(undefined);

    dispose = initPendingLaunch();
    await flush();
    dispose();
    passSignIn();
    loadLibrary();
    await flush();

    expect(play).not.toHaveBeenCalled();
  });
});

describe('an emulator prompt left by a launch outside the window', () => {
  it('is asked at mount before the window is ready, and its answer resumes the game\'s own launch', async () => {
    takePending.mockResolvedValueOnce({ gameId: 'g1', platform: 'ps2', code: 'emulators.not_configured', profileId: 'ab12cd34', profileLaunch: true });
    play.mockResolvedValue(undefined);

    dispose = initPendingLaunch();
    await vi.waitFor(() => expect(get(emulatorPrompt)?.platform).toBe('ps2'));
    get(emulatorPrompt)!.settle(true);

    await vi.waitFor(() => expect(play).toHaveBeenCalledWith('g1'));
    expect(playProfile).not.toHaveBeenCalled();
    expect(get(toasts)).toEqual([]);
  });
});

describe('the kind of a pending launch, told by its code', () => {
  const others = [
    { name: 'a code that is neither, with no platform', pending: { gameId: 'g1', platform: '', code: 'library.game_not_found', profileId: '', profileLaunch: false } },
    { name: 'a code that is neither, with a platform', pending: { gameId: 'g1', platform: 'ps2', code: 'library.game_not_found', profileId: '', profileLaunch: false } },
  ];

  it.each(others)('$name: logged and dropped, nothing asked or launched', async ({ pending }) => {
    takePending.mockResolvedValueOnce(pending);
    play.mockResolvedValue(undefined);
    playProfile.mockResolvedValue(undefined);

    dispose = initPendingLaunch();
    await flush();
    passSignIn();
    loadLibrary();
    await flush();

    expect(get(emulatorPrompt)).toBeNull();
    expect(play).not.toHaveBeenCalled();
    expect(playProfile).not.toHaveBeenCalled();
    expect(get(cloudConflictTarget)).toBeNull();
    expect(toastList()).toEqual([]);
    expect(console.error).toHaveBeenCalledWith(expect.any(String), 'library.game_not_found');
  });

  it('an emulator code asks for the emulator even with the window ready', async () => {
    takePending.mockResolvedValueOnce({ gameId: 'g1', platform: 'ps2', code: 'emulators.file_missing', profileId: '', profileLaunch: false });
    passSignIn();
    loadLibrary();

    dispose = initPendingLaunch();

    await vi.waitFor(() => expect(get(emulatorPrompt)?.code).toBe('emulators.file_missing'));
    expect(play).not.toHaveBeenCalled();
  });

  it('a cloud saves code resumes the launch', async () => {
    takePending.mockResolvedValueOnce(heldBack('cloud.syncing', entries[0].held));
    play.mockResolvedValue(undefined);
    passSignIn();
    loadLibrary();

    dispose = initPendingLaunch();

    await vi.waitFor(() => expect(play).toHaveBeenCalledWith('g1'));
    expect(get(emulatorPrompt)).toBeNull();
  });
});
