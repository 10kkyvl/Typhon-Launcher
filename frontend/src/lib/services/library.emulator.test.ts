import { beforeEach, describe, expect, it, vi } from 'vitest';

const { play, games } = vi.hoisted(() => ({ play: vi.fn(), games: vi.fn() }));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { PlayGame: play, GetGames: games } }));
vi.mock('./backend', () => ({ inWails: true }));

import { get } from 'svelte/store';
import { launchGame, playGame } from './library';
import { emulatorPrompt } from '../stores/emulatorPrompt';

const needs = (code: string) => new Error(`typhon:${code}: detail`);

async function prompt() {
  await vi.waitFor(() => expect(get(emulatorPrompt)).not.toBeNull());
  return get(emulatorPrompt)!;
}

beforeEach(() => {
  play.mockReset();
  games.mockReset();
  emulatorPrompt.set(null);
  games.mockResolvedValue([
    { id: 'rom', emulation: { platform: 'ps2' } },
    { id: 'pc' },
  ]);
});

describe('launching a game that needs an emulator', () => {
  it('asks for the console of the game and launches again once it is set', async () => {
    play.mockRejectedValueOnce(needs('emulators.not_configured')).mockResolvedValueOnce(undefined);
    const launched = launchGame('rom');
    const asked = await prompt();
    expect(asked.platform).toBe('ps2');
    expect(asked.code).toBe('emulators.not_configured');
    asked.settle(true);
    await expect(launched).resolves.toBe(true);
    expect(play).toHaveBeenCalledTimes(2);
    expect(get(emulatorPrompt)).toBeNull();
  });

  it.each([
    'emulators.file_missing',
    'emulators.windows_program',
    'emulators.not_exe',
    'emulators.not_regular_file',
    'emulators.file_unreadable',
    'emulators.path_relative',
  ])('asks on %s too and keeps the reason', async (code) => {
    play.mockRejectedValueOnce(needs(code));
    const launched = launchGame('rom');
    const asked = await prompt();
    expect(asked.code).toBe(code);
    asked.settle(false);
    await expect(launched).resolves.toBe(false);
  });

  it('does not launch when the dialog is cancelled', async () => {
    play.mockRejectedValueOnce(needs('emulators.not_configured'));
    const launched = launchGame('rom');
    (await prompt()).settle(false);
    await expect(launched).resolves.toBe(false);
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('shows the second failure instead of asking again', async () => {
    const second = needs('emulators.not_exe');
    play.mockRejectedValueOnce(needs('emulators.not_configured')).mockRejectedValueOnce(second);
    const launched = playGame('rom');
    (await prompt()).settle(true);
    await expect(launched).rejects.toBe(second);
    expect(get(emulatorPrompt)).toBeNull();
  });

  it('keeps a cancelled native launch after the dialog silent', async () => {
    play
      .mockRejectedValueOnce(needs('emulators.not_configured'))
      .mockRejectedValueOnce(new Error('typhon:library.launch_cancelled: context canceled'));
    const launched = launchGame('rom');
    (await prompt()).settle(true);
    await expect(launched).resolves.toBe(false);
  });

  it('reports the original error when the game has no console', async () => {
    const failure = needs('emulators.not_configured');
    play.mockRejectedValueOnce(failure);
    await expect(launchGame('pc')).rejects.toBe(failure);
    expect(get(emulatorPrompt)).toBeNull();
  });

  it('does not touch other failures', async () => {
    const failure = needs('library.launch_failed');
    play.mockRejectedValueOnce(failure);
    await expect(launchGame('rom')).rejects.toBe(failure);
    expect(games).not.toHaveBeenCalled();
  });
});
