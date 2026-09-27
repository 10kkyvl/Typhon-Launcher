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
} as const;

export type InstalledKey = keyof typeof installed;
