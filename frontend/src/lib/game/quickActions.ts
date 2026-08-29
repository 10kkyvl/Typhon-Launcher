export type QuickAction =
  | 'play'
  | 'stop'
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
}

export function quickActions(state: QuickActionState): QuickActionItem[] {
  const items: QuickActionItem[] = [];
  if (!state.installed) {
    return [{ id: 'remove', label: 'Удалить из библиотеки', danger: true }];
  }
  if (state.running) {
    items.push({ id: 'stop', label: 'Остановить' });
  } else if (state.hasExecutable) {
    items.push({ id: 'play', label: 'Играть' });
  }
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
