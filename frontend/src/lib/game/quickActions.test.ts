import { describe, expect, it } from 'vitest';
import { quickActions, type QuickAction, type QuickActionState } from './quickActions';

const state = (over: Partial<QuickActionState> = {}): QuickActionState => ({
  installed: true,
  running: false,
  hasExecutable: true,
  hasShortcut: false,
  lanEnabled: false,
  lanShared: false,
  favorite: false,
  status: '',
  ...over,
});

const ids = (over: Partial<QuickActionState> = {}): QuickAction[] =>
  quickActions(state(over)).map((item) => item.id);

describe('quickActions', () => {
  it('offers the full set for an installed game', () => {
    expect(ids()).toEqual([
      'play',
      'favorite-add',
      'status',
      'folder',
      'saves',
      'verify',
      'move',
      'shortcut-create',
      'uninstall',
      'remove',
    ]);
  });

  it('shows the status item for an installed game', () => {
    const items = quickActions(state());
    const item = items.find((i) => i.id === 'status');
    expect(item?.label).toBe('Статус: Без статуса');
  });

  it('shows the status item for a game that is not installed', () => {
    const items = quickActions(state({ installed: false }));
    const item = items.find((i) => i.id === 'status');
    expect(item?.label).toBe('Статус: Без статуса');
  });

  it('reflects the current status in the label', () => {
    const items = quickActions(state({ status: 'completed' }));
    const item = items.find((i) => i.id === 'status');
    expect(item?.label).toBe('Статус: Пройдена');
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

  it('keeps marks and library removal for a game that is not installed', () => {
    expect(ids({ installed: false })).toEqual(['favorite-add', 'status', 'remove']);
  });

  it('flips the favorite item once set', () => {
    expect(ids({ favorite: true })).toContain('favorite-remove');
    expect(ids({ favorite: true })).not.toContain('favorite-add');
  });

  it('marks both removals as dangerous', () => {
    const danger = quickActions(state())
      .filter((item) => item.danger)
      .map((item) => item.id);
    expect(danger).toEqual(['uninstall', 'remove']);
  });

  it('hides the move action while the game is running', () => {
    expect(ids({ running: true })).not.toContain('move');
  });

  it('hides local network sharing until the setting is on', () => {
    expect(ids()).not.toContain('lan-share');
    expect(ids({ lanEnabled: true })).toContain('lan-share');
  });

  it('offers to stop sharing a game that is already shared', () => {
    const shown = ids({ lanEnabled: true, lanShared: true });
    expect(shown).toContain('lan-unshare');
    expect(shown).not.toContain('lan-share');
  });

  it('ignores lan sharing for a game that is not installed', () => {
    expect(ids({ installed: false, lanEnabled: true })).toEqual(['favorite-add', 'status', 'remove']);
  });
});
