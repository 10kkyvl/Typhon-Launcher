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
vi.mock('../../../bindings/typhon/emulatorpromptservice', () => ({ TakePending: takePending }));
vi.mock('../services/backend', () => ({ inWails: true }));

import { get } from 'svelte/store';
import type { LibraryGame } from '../services/library';
import { closeCloudConflict } from './cloudsaves';
import { initEmulatorNeeded } from './emulatorNeeded';
import { emulatorPrompt } from './emulatorPrompt';
import { libraryGames } from './library';
import { toasts } from './toasts';

const nothing = { gameId: '', platform: '', code: '', profileId: '', profileLaunch: false };

let dispose: () => void = () => {};

beforeEach(() => {
  handlers.length = 0;
  play.mockReset();
  playProfile.mockReset();
  games.mockReset().mockResolvedValue([{ id: 'g1', emulation: { platform: 'ps2' } }]);
  takePending.mockReset().mockResolvedValue(nothing);
  closeCloudConflict();
  emulatorPrompt.set(null);
  toasts.set([]);
  libraryGames.set([]);
});

afterEach(() => {
  dispose();
  get(emulatorPrompt)?.settle(false);
});

describe('an emulator prompt left by a launch outside the window', () => {
  it('is asked at mount before the window is ready, and its answer resumes the game\'s own launch', async () => {
    takePending.mockResolvedValueOnce({ gameId: 'g1', platform: 'ps2', code: 'emulators.not_configured', profileId: 'ab12cd34', profileLaunch: true });
    play.mockResolvedValue(undefined);

    dispose = initEmulatorNeeded();
    await vi.waitFor(() => expect(get(emulatorPrompt)?.platform).toBe('ps2'));
    get(emulatorPrompt)!.settle(true);

    await vi.waitFor(() => expect(play).toHaveBeenCalledWith('g1'));
    expect(playProfile).not.toHaveBeenCalled();
    expect(get(toasts)).toEqual([]);
  });
});
