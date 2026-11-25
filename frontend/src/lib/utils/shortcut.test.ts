import { describe, expect, it } from 'vitest';
import { composing, onMacKeyboard, searchShortcut } from './shortcut';

describe('searchShortcut', () => {
  it('shows Command on a Mac and Ctrl elsewhere', () => {
    expect(onMacKeyboard('MacIntel')).toBe(true);
    expect(onMacKeyboard('Win32')).toBe(false);
    expect(onMacKeyboard('Linux x86_64')).toBe(false);
    expect(onMacKeyboard('')).toBe(false);
    expect(searchShortcut(true)).toBe('⌘K');
    expect(searchShortcut(false)).toBe('Ctrl K');
  });
});

describe('composing', () => {
  const key = (init: { isComposing?: boolean; keyCode?: number }) => ({ key: 'Enter', isComposing: false, keyCode: 13, ...init }) as KeyboardEvent;

  it('takes a key of an input method for the input method: while composing, and the Enter WebKit sends after it', () => {
    expect(composing(key({ isComposing: true }))).toBe(true);
    expect(composing(key({ keyCode: 229 }))).toBe(true);
    expect(composing(key({}))).toBe(false);
  });
});
