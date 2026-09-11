import { describe, expect, it, vi } from 'vitest';
const { play } = vi.hoisted(() => ({ play: vi.fn() }));
vi.mock('../../../bindings/typhon/internal/library', () => ({ Service: { PlayGame: play } }));
vi.mock('./backend', () => ({ inWails: true }));
import { playGame } from './library';
describe('launch cancellation at the UI boundary', () => {
  it('silences a user cancellation', async () => {
    play.mockRejectedValueOnce(new Error('typhon:library.launch_cancelled: context canceled'));
    await expect(playGame('g')).resolves.toBeUndefined();
  });
  it('preserves actual launch failures for the toast', async () => {
    const failure = new Error('typhon:library.launch_failed: failure');
    play.mockRejectedValueOnce(failure);
    await expect(playGame('g')).rejects.toBe(failure);
  });
});
