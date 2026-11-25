export const installed = {
  'installed.youHave': {
    one: 'У вас установлена {count} игра',
    few: 'У вас установлено {count} игры',
    many: 'У вас установлено {count} игр',
  },
  'installed.shownOf': {
    one: 'Показано {shown} из {count} игры',
    few: 'Показано {shown} из {count} игр',
    many: 'Показано {shown} из {count} игр',
  },
  // 7-C2 installed
  'installed.storageLoading': 'Читаем данные о диске…',
  'installed.meterDisk': 'Диск {volume}',
  'installed.meterMoreGames': {
    one: 'Ещё {count} игра',
    few: 'Ещё {count} игры',
    many: 'Ещё {count} игр',
  },
  'installed.moving': 'Переносится',
  'installed.menuMoving': '{action}: идёт перенос',
  // /7-C2 installed
} as const;

export type InstalledKey = keyof typeof installed;
