export const format = {
  'units.b': 'Б',
  'units.kb': 'КБ',
  'units.mb': 'МБ',
  'units.gb': 'ГБ',
  'units.tb': 'ТБ',
  'units.kbs': 'КБ/с',
  'units.mbs': 'МБ/с',
  'units.hour': 'ч',
  'units.minute': 'мин',
  'units.second': 'сек',
  'format.noLimit': 'Без ограничений',
  'format.lessThanMinute': 'меньше минуты',
  'format.today': 'Сегодня',
  'format.yesterday': 'Вчера',
  'format.daysAgo': { one: '{count} день назад', few: '{count} дня назад', many: '{count} дней назад' },
  'format.weeksAgo': { one: '{count} неделю назад', few: '{count} недели назад', many: '{count} недель назад' },
} as const;

export type FormatKey = keyof typeof format;
