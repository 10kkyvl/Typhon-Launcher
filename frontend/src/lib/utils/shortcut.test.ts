import { describe, expect, it } from 'vitest';
import { onMacKeyboard, searchShortcut } from './shortcut';

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
