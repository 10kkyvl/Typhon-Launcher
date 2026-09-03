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
      ? { id: 'favorite-remove', label: 'Убрать из любимых' }
      : { id: 'favorite-add', label: 'В любимые' },
    { id: 'status', label: `Статус: ${statusLabel(state.status)}` },
  ];
}

export function quickActions(state: QuickActionState): QuickActionItem[] {
  const items: QuickActionItem[] = [];
  if (!state.installed) {
    return [...markItems(state), { id: 'remove', label: 'Удалить из библиотеки', danger: true, separator: true }];
  }
  if (state.running) {
    items.push({ id: 'stop', label: 'Остановить' });
  } else if (state.hasExecutable) {
    items.push({ id: 'play', label: 'Играть' });
  }
  items.push(...markItems(state));
  items.push({ id: 'folder', label: 'Открыть папку' });
  items.push({ id: 'saves', label: 'Открыть сохранения' });
  items.push({ id: 'verify', label: 'Проверить файлы' });
  if (!state.running) {
    items.push({ id: 'move', label: 'Переместить на другой диск' });
  }
  if (state.lanEnabled) {
    items.push(
      state.lanShared
        ? { id: 'lan-unshare', label: 'Не раздавать в локальной сети' }
        : { id: 'lan-share', label: 'Раздать в локальной сети' },
    );
  }
  items.push(
    state.hasShortcut
      ? { id: 'shortcut-remove', label: 'Удалить ярлык с рабочего стола' }
      : { id: 'shortcut-create', label: 'Создать ярлык на рабочем столе' },
  );
  items.push({ id: 'uninstall', label: 'Удалить с компьютера', danger: true, separator: true });
  items.push({ id: 'remove', label: 'Удалить из библиотеки', danger: true });
  return items;
}
