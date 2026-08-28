import { describe, expect, it } from 'vitest';
import { quickActions, type QuickAction, type QuickActionState } from './quickActions';

const state = (over: Partial<QuickActionState> = {}): QuickActionState => ({
  installed: true,
  running: false,
  hasExecutable: true,
  hasShortcut: false,
  ...over,
});

const ids = (over: Partial<QuickActionState> = {}): QuickAction[] =>
  quickActions(state(over)).map((item) => item.id);

describe('quickActions', () => {
  it('offers the full set for an installed game', () => {
    expect(ids()).toEqual([
      'play',
      'folder',
      'saves',
      'verify',
      'shortcut-create',
      'uninstall',
      'remove',
    ]);
  });

  it('replaces play with stop while the game runs', () => {
    expect(ids({ running: true })).toContain('stop');
    expect(ids({ running: true })).not.toContain('play');
  });

  it('drops play when there is nothing to launch', () => {
    expect(ids({ hasExecutable: false })).not.toContain('play');
    expect(ids({ hasExecutable: false })).toContain('folder');
  });

  it('switches the shortcut item once a shortcut exists', () => {
    expect(ids({ hasShortcut: true })).toContain('shortcut-remove');
    expect(ids({ hasShortcut: true })).not.toContain('shortcut-create');
  });

  it('leaves only library removal for a game that is not installed', () => {
    expect(ids({ installed: false })).toEqual(['remove']);
  });

  it('marks both removals as dangerous', () => {
    const danger = quickActions(state())
      .filter((item) => item.danger)
      .map((item) => item.id);
    expect(danger).toEqual(['uninstall', 'remove']);
  });
});
