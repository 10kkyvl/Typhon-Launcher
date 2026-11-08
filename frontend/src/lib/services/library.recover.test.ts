import { beforeEach, describe, expect, it, vi } from 'vitest';

const { play, games } = vi.hoisted(() => ({ play: vi.fn(), games: vi.fn() }));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { PlayGame: play, GetGames: games } }));
vi.mock('./backend', () => ({ inWails: true }));

import { get } from 'svelte/store';
import { launchGame } from './library';
import { biosPrompt } from '../stores/biosPrompt';
import { emulatorBusyPrompt } from '../stores/emulatorBusyPrompt';
import { emulatorPrompt } from '../stores/emulatorPrompt';
import { toasts } from '../stores/toasts';

const fails = (code: string) => new Error(`typhon:${code}: detail`);

async function shown<T>(store: { subscribe: (run: (value: T | null) => void) => () => void }): Promise<T> {
  await vi.waitFor(() => expect(get(store)).not.toBeNull());
  return get(store)!;
}

beforeEach(() => {
  play.mockReset();
  games.mockReset();
  biosPrompt.set(null);
  emulatorBusyPrompt.set(null);
  emulatorPrompt.set(null);
  toasts.set([]);
  games.mockResolvedValue([
    { id: 'ps3', emulation: { platform: 'ps3' } },
    { id: 'ps2', emulation: { platform: 'ps2' } },
  ]);
});

describe('a launch retried after a question', () => {
  it('shows the package installer as a notice after the emulator was closed', async () => {
    play.mockRejectedValueOnce(fails('emulators.already_running')).mockRejectedValueOnce(fails('emulators.pkg_installing'));
    const launched = launchGame('ps3');
    (await shown(emulatorBusyPrompt)).settle(true);
    await expect(launched).resolves.toBe(false);
    expect(get(toasts).map((item) => item.kind)).toEqual(['info']);
  });

  it('shows the package installer as a notice after the emulator was chosen', async () => {
    play.mockRejectedValueOnce(fails('emulators.not_configured')).mockRejectedValueOnce(fails('emulators.pkg_installing'));
    const launched = launchGame('ps3');
    (await shown(emulatorPrompt)).settle(true);
    await expect(launched).resolves.toBe(false);
    expect(get(toasts).map((item) => item.kind)).toEqual(['info']);
  });

  it('offers to close the emulator after the BIOS was put in place', async () => {
    play
      .mockRejectedValueOnce(fails('emulators.bios_missing'))
      .mockRejectedValueOnce(fails('emulators.already_running'))
      .mockResolvedValueOnce(undefined);
    const launched = launchGame('ps2');
    (await shown(biosPrompt)).settle(true);
    (await shown(emulatorBusyPrompt)).settle(true);
    await expect(launched).resolves.toBe(true);
    expect(play).toHaveBeenCalledTimes(3);
  });

  it('asks for the BIOS after the emulator was chosen', async () => {
    play
      .mockRejectedValueOnce(fails('emulators.not_configured'))
      .mockRejectedValueOnce(fails('emulators.bios_missing'))
      .mockRejectedValueOnce(fails('emulators.bios_missing'))
      .mockResolvedValueOnce(undefined);
    const launched = launchGame('ps2');
    (await shown(emulatorPrompt)).settle(true);
    const first = await shown(biosPrompt);
    expect(first.stillMissing).toBe(false);
    first.settle(true);
    await vi.waitFor(() => expect(get(biosPrompt)?.stillMissing).toBe(true));
    get(biosPrompt)!.settle(true);
    await expect(launched).resolves.toBe(true);
  });

  it('shows a copy that is open again instead of asking twice', async () => {
    const second = fails('emulators.already_running');
    play.mockRejectedValueOnce(fails('emulators.already_running')).mockRejectedValueOnce(second);
    const launched = launchGame('ps3');
    (await shown(emulatorBusyPrompt)).settle(true);
    await expect(launched).rejects.toBe(second);
    expect(get(emulatorBusyPrompt)).toBeNull();
  });

  it('gives the launch up when the player keeps the emulator open', async () => {
    play.mockRejectedValueOnce(fails('emulators.already_running'));
    const launched = launchGame('ps3');
    (await shown(emulatorBusyPrompt)).settle(false);
    await expect(launched).resolves.toBe(false);
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('gives the launch up when the BIOS prompt is closed', async () => {
    play.mockRejectedValueOnce(fails('emulators.bios_missing'));
    const launched = launchGame('ps2');
    (await shown(biosPrompt)).settle(false);
    await expect(launched).resolves.toBe(false);
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('shows any other failure of the retry as it is', async () => {
    const failure = fails('library.launch_failed');
    play.mockRejectedValueOnce(fails('emulators.bios_missing')).mockRejectedValueOnce(failure);
    const launched = launchGame('ps2');
    (await shown(biosPrompt)).settle(true);
    await expect(launched).rejects.toBe(failure);
  });
});
