import { describe, expect, it } from 'vitest';
import { canPlay } from './launch';

describe('canPlay', () => {
  it.each([
    [{ executable: 'game.exe' }, true],
    [{ executable: 'game.exe', uninstalled: false }, true],
    [{ executable: '' }, false],
    [{ executable: '', uninstalled: false }, false],
    [{ executable: 'game.exe', uninstalled: true }, false],
  ])('%j gives %s', (game, expected) => {
    expect(canPlay(game)).toBe(expected);
  });
});
