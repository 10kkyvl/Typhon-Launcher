export const settings = {
  'settings.language': 'Язык',
  'settings.languageSub': 'Язык интерфейса лаунчера',
  'settings.languageSystem': 'Системный',
} as const;

export type SettingsKey = keyof typeof settings;
