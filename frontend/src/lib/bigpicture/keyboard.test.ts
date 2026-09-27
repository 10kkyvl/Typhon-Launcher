import { describe, expect, it } from 'vitest';
import { appendText, eraseCharacter, keyboardRows } from './keyboard';

describe('Big Picture keyboard text', () => {
  it('can enter Windows paths and magnet links without a physical keyboard', () => {
    const available = new Set([...keyboardRows.en.join(''), ' ']);
    for (const input of [String.raw`C:\Games\Pilot's Adventure (2026)\game.exe`, 'magnet:?xt=urn:btih:0123abcd&dn=Silver%20Harbor']) {
      expect([...input.toLowerCase()].every((character) => available.has(character))).toBe(true);
    }
  });

  it('keeps pasted Unicode complete when limiting or erasing characters', () => {
    expect(appendText('Иг', 'ра🎮 ещё', 5)).toBe('Игра🎮');
    expect(eraseCharacter('Игра🎮')).toBe('Игра');
    expect(eraseCharacter('')).toBe('');
  });
  it('does not grow past the caller limit and supports clearing then retrying input', () => {
    expect(appendText('abc', 'd', 3)).toBe('abc');
    expect(appendText(eraseCharacter('abc'), 'z', 3)).toBe('abz');
    expect(appendText('', 'new', 3)).toBe('new');
  });
});
