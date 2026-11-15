export const downloads = {
  'downloads.selected': {
    one: 'Выбрано: {count} файл, {size}',
    few: 'Выбрано: {count} файла, {size}',
    many: 'Выбрано: {count} файлов, {size}',
  },
  'downloads.active': { one: '{count} активная', few: '{count} активные', many: '{count} активных' },
  'downloads.speedGraph.caption': 'Скорость за последние {seconds} сек',
  'downloads.speedGraph.peak': 'пик {speed}',
  'downloads.speedGraph.ago': '−{seconds} сек',
} as const;

export type DownloadsKey = keyof typeof downloads;
