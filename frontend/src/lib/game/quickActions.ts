import { msg } from '../i18n';
import { statusLabel } from './status';

export type QuickAction =
  | 'play'
  | 'stop'
  | 'favorite-add'
  | 'favorite-remove'
  | 'status'
  | 'folder'
  | 'saves'
  | 'verify'
  | 'move'
  | 'lan-share'
  | 'lan-unshare'
  | 'shortcut-create'
  | 'shortcut-remove'
  | 'uninstall'
  | 'remove';

export interface QuickActionItem {
  id: QuickAction;
  label: string;
  danger?: boolean;
  separator?: boolean;
}

export interface QuickActionState {
  installed: boolean;
  running: boolean;
  hasExecutable: boolean;
  hasShortcut: boolean;
  lanEnabled: boolean;
  lanShared: boolean;
  favorite: boolean;
  status: string;
}

function markItems(state: QuickActionState): QuickActionItem[] {
  return [
    state.favorite
      ? { id: 'favorite-remove', label: msg('games.actionFavoriteRemove') }
      : { id: 'favorite-add', label: msg('games.actionFavoriteAdd') },
    { id: 'status', label: msg('games.actionStatus', { status: statusLabel(state.status) }) },
  ];
}

export function quickActions(state: QuickActionState): QuickActionItem[] {
  const items: QuickActionItem[] = [];
  if (!state.installed) {
    return [
      ...markItems(state),
      { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true, separator: true },
    ];
  }
  if (state.running) {
    items.push({ id: 'stop', label: msg('games.stop') });
  } else if (state.hasExecutable) {
    items.push({ id: 'play', label: msg('games.play') });
  }
  items.push(...markItems(state));
  items.push({ id: 'folder', label: msg('games.openFolder') });
  items.push({ id: 'saves', label: msg('games.actionOpenSaves') });
  items.push({ id: 'verify', label: msg('games.actionVerify') });
  if (!state.running) {
    items.push({ id: 'move', label: msg('games.actionMove') });
  }
  if (state.lanEnabled) {
    items.push(
      state.lanShared
        ? { id: 'lan-unshare', label: msg('games.actionLanUnshare') }
        : { id: 'lan-share', label: msg('games.actionLanShare') },
    );
  }
  items.push(
    state.hasShortcut
      ? { id: 'shortcut-remove', label: msg('games.actionShortcutRemove') }
      : { id: 'shortcut-create', label: msg('games.actionShortcutCreate') },
  );
  items.push({ id: 'uninstall', label: msg('games.actionUninstall'), danger: true, separator: true });
  items.push({ id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true });
  return items;
}
