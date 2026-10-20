export const errGameMode = {
  'gamemode.hide_needs_tray': 'Игровой режим: скрыть лаунчер не вышло, включите «Сворачивать в трей» в настройках',
  'gamemode.power_scheme_missing': 'Игровой режим: в Windows нет схемы питания «Высокая производительность»',
  'gamemode.power_failed': 'Игровой режим: не удалось переключить схему питания',
  'gamemode.priority_failed': 'Игровой режим: не удалось повысить приоритет игры. Возможно, она запущена от администратора',
  'gamemode.unsupported': 'Игровой режим: это действие не поддерживается на вашей системе',
  'gamemode.marker_write_failed': 'Игровой режим: не удалось записать данные для возврата схемы питания, схема не изменена',
  'gamemode.marker_corrupt': 'Игровой режим: файл возврата схемы питания повреждён, схема питания не менялась',
  'gamemode.restore_failed': 'Игровой режим: не удалось вернуть прежнюю схему питания, проверьте её в настройках Windows',
  'gamemode.failed': 'Игровой режим: не удалось применить настройки',
} as const;

export type ErrGameModeKey = keyof typeof errGameMode;
