import { describe, expect, it, vi } from 'vitest';
import type { GameModeOverride } from '../services/library';
import { held } from '../testing/gameDialogsTesting';
import { createGameModeWrites } from './gameModeWrites.svelte';

function setup(copy: GameModeOverride | null = null) {
  const library = { copy };
  const send = vi.fn<(gameId: string, override: GameModeOverride) => Promise<unknown>>();
  const writes = createGameModeWrites(send, () => library.copy);
  return { library, send, writes };
}

describe('createGameModeWrites', () => {
  it('builds a change on the last one Go took until the library holds it, then on the library', async () => {
    const { library, send, writes } = setup({ raisePriority: true });
    send.mockResolvedValue(undefined);

    await writes.write('g1', 'highPerformance', 'on');
    library.copy = { raisePriority: false };
    writes.settle();
    await writes.write('g1', 'hideLauncher', 'off');
    library.copy = { raisePriority: true, highPerformance: true, hideLauncher: false };
    writes.settle();
    await writes.write('g1', 'highPerformance', 'inherit');

    expect(send.mock.calls).toEqual([
      ['g1', { highPerformance: true, raisePriority: true }],
      ['g1', { highPerformance: true, hideLauncher: false, raisePriority: true }],
      ['g1', { hideLauncher: false, raisePriority: true }],
    ]);
  });

  it("does not let an older answer clear a newer choice of the same row", async () => {
    const { send, writes } = setup();
    const first = held<unknown>();
    const second = held<unknown>();
    send.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const one = writes.write('g1', 'hideLauncher', 'on');
    const two = writes.write('g1', 'hideLauncher', 'off');
    first.reject(new Error('disk'));
    await expect(one).rejects.toThrow('disk');

    expect(writes.chosen('hideLauncher')).toBe('off');
    await vi.waitFor(() => expect(send).toHaveBeenCalledTimes(2));
    second.resolve(undefined);
    await two;
    expect(writes.chosen('hideLauncher')).toBe('off');
    writes.settle();
    expect(writes.chosen('hideLauncher')).toBeUndefined();
  });

  it('after a reset, still sends what was queued but shows nothing of it', async () => {
    const { send, writes } = setup();
    const first = held<unknown>();
    send.mockReturnValueOnce(first.promise).mockResolvedValueOnce(undefined);

    void writes.write('g1', 'hideLauncher', 'on');
    const queued = writes.write('g1', 'raisePriority', 'off');
    writes.reset();
    expect(writes.waiting('g1')).toBe(2);
    first.resolve(undefined);
    await queued;

    expect(send.mock.calls).toEqual([
      ['g1', { hideLauncher: true }],
      ['g1', { hideLauncher: true, raisePriority: false }],
    ]);
    expect(writes.chosen('hideLauncher')).toBeUndefined();
    expect(writes.chosen('raisePriority')).toBeUndefined();
    expect(writes.waiting('g1')).toBe(0);
  });
});
