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
  hidden: false,
  saveBackups: true,
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
      'tags',
      'hide',
      'folder',
      'saves',
      'saves-backups',
      'verify',
      'redist',
      'move',
      'properties',
      'shortcut-create',
      'uninstall',
      'remove',
    ]);
  });

  it('hides the saves window while backups are switched off', () => {
    expect(ids({ saveBackups: false })).not.toContain('saves-backups');
    expect(ids({ saveBackups: false })).toContain('saves');
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

  it('offers show instead of hide for a hidden game', () => {
    expect(ids({ hidden: true })).toContain('unhide');
    expect(ids({ hidden: true })).not.toContain('hide');
    expect(ids({ hidden: true, installed: false })).toContain('unhide');
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
    expect(ids({ installed: false })).toEqual(['favorite-add', 'status', 'tags', 'hide', 'remove']);
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

  it('offers properties only for an installed game', () => {
    expect(ids()).toContain('properties');
    expect(ids({ installed: false })).not.toContain('properties');
  });

  it('offers runtime libraries only for an installed game', () => {
    expect(ids()).toContain('redist');
    expect(ids({ running: true })).toContain('redist');
    expect(ids({ installed: false })).not.toContain('redist');
    expect(quickActions(state()).find((i) => i.id === 'redist')?.label).toBe('Библиотеки среды');
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
    expect(ids({ installed: false, lanEnabled: true })).toEqual(['favorite-add', 'status', 'tags', 'hide', 'remove']);
  });

  describe('steam item', () => {
    it('stays hidden until the status arrives', () => {
      expect(ids()).not.toContain('steam-add');
      expect(ids()).not.toContain('steam-remove');
      expect(ids({ steam: null })).not.toContain('steam-add');
    });

    it('offers to add a game that is not in steam yet', () => {
      const list = ids({ steam: { added: false } });
      expect(list).toContain('steam-add');
      expect(list).not.toContain('steam-remove');
      expect(quickActions(state({ steam: { added: false } })).find((i) => i.id === 'steam-add')?.label).toBe(
        'Добавить в Steam',
      );
    });

    it('offers to remove a game that is already in steam', () => {
      const list = ids({ steam: { added: true } });
      expect(list).toContain('steam-remove');
      expect(list).not.toContain('steam-add');
      expect(quickActions(state({ steam: { added: true } })).find((i) => i.id === 'steam-remove')?.label).toBe(
        'Убрать из Steam',
      );
    });

    it('hides for a game that comes from steam', () => {
      expect(ids({ steam: { added: false }, external: true, fromSteam: true })).not.toContain('steam-add');
    });

    it('hides for a game that is not installed or has no executable', () => {
      expect(ids({ steam: { added: false }, installed: false })).not.toContain('steam-add');
      expect(ids({ steam: { added: false }, hasExecutable: false })).not.toContain('steam-add');
    });

    it('shows for a game imported from another launcher', () => {
      expect(ids({ steam: { added: false }, external: true })).toContain('steam-add');
    });
  });

  describe('external game', () => {
    const external = { external: true, hasExecutable: false, launchable: true };

    it('plays through the launch uri without an executable', () => {
      expect(ids(external)).toContain('play');
      expect(ids({ hasExecutable: false })).not.toContain('play');
    });

    it('drops the actions the other launcher owns', () => {
      const list = ids(external);
      expect(list).not.toContain('verify');
      expect(list).not.toContain('move');
      expect(list).not.toContain('uninstall');
      expect(list).not.toContain('shortcut-create');
    });

    it('keeps folder, marks and removal from the library', () => {
      const list = ids(external);
      expect(list).toEqual(expect.arrayContaining(['folder', 'favorite-add', 'status', 'tags', 'hide', 'remove']));
    });

    it('offers a shortcut when the executable is known', () => {
      expect(ids({ ...external, hasExecutable: true })).toContain('shortcut-create');
    });

    it('keeps stop while running', () => {
      expect(ids({ ...external, running: true })).toContain('stop');
    });
  });
});
