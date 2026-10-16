import { msg } from '../i18n';
import { statusLabel } from './status';

export type QuickAction =
  | 'play'
  | 'stop'
  | 'favorite-add'
  | 'favorite-remove'
  | 'status'
  | 'tags'
  | 'hide'
  | 'unhide'
  | 'folder'
  | 'saves'
  | 'saves-backups'
  | 'verify'
  | 'redist'
  | 'move'
  | 'properties'
  | 'lan-share'
  | 'lan-unshare'
  | 'shortcut-create'
  | 'shortcut-remove'
  | 'steam-add'
  | 'steam-remove'
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
  launchable?: boolean;
  external?: boolean;
  hasShortcut: boolean;
  lanEnabled: boolean;
  lanShared: boolean;
  favorite: boolean;
  status: string;
  hidden: boolean;
  saveBackups: boolean;
  fromSteam?: boolean;
  steam?: { added: boolean } | null;
}

export function steamEligible(state: Pick<QuickActionState, 'installed' | 'hasExecutable' | 'fromSteam'>): boolean {
  return state.installed && state.hasExecutable && !state.fromSteam;
}

function markItems(state: QuickActionState): QuickActionItem[] {
  return [
    state.favorite
      ? { id: 'favorite-remove', label: msg('games.actionFavoriteRemove') }
      : { id: 'favorite-add', label: msg('games.actionFavoriteAdd') },
    { id: 'status', label: msg('games.actionStatus', { status: statusLabel(state.status) }) },
    { id: 'tags', label: msg('games.actionTags') },
    state.hidden
      ? { id: 'unhide', label: msg('games.actionUnhide') }
      : { id: 'hide', label: msg('games.actionHide') },
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
  } else if (state.launchable ?? state.hasExecutable) {
    items.push({ id: 'play', label: msg('games.play') });
  }
  items.push(...markItems(state));
  items.push({ id: 'folder', label: msg('games.openFolder') });
  items.push({ id: 'saves', label: msg('games.actionOpenSaves') });
  if (state.saveBackups) {
    items.push({ id: 'saves-backups', label: msg('games.actionSavesBackups') });
  }
  if (!state.external) {
    items.push({ id: 'verify', label: msg('games.actionVerify') });
  }
  items.push({ id: 'redist', label: msg('redist.action') });
  if (!state.running && !state.external) {
    items.push({ id: 'move', label: msg('games.actionMove') });
  }
  items.push({ id: 'properties', label: msg('games.actionProperties') });
  if (state.lanEnabled) {
    items.push(
      state.lanShared
        ? { id: 'lan-unshare', label: msg('games.actionLanUnshare') }
        : { id: 'lan-share', label: msg('games.actionLanShare') },
    );
  }
  if (state.hasShortcut) {
    items.push({ id: 'shortcut-remove', label: msg('games.actionShortcutRemove') });
  } else if (state.hasExecutable || !state.external) {
    items.push({ id: 'shortcut-create', label: msg('games.actionShortcutCreate') });
  }
  if (state.steam && steamEligible(state)) {
    items.push(
      state.steam.added
        ? { id: 'steam-remove', label: msg('games.actionSteamRemove') }
        : { id: 'steam-add', label: msg('games.actionSteamAdd') },
    );
  }
  if (state.external) {
    items.push({ id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true, separator: true });
    return items;
  }
  items.push({ id: 'uninstall', label: msg('games.actionUninstall'), danger: true, separator: true });
  items.push({ id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true });
  return items;
}
