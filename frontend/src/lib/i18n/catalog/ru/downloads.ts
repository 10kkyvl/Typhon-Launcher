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

  // ---- 6-3: the panel of a download under way (lib/components/DownloadItem.svelte)
  'downloads.panel.pause': 'Пауза',
  'downloads.panel.pauseNamed': 'Пауза: «{name}»',
  'downloads.panel.resume': 'Продолжить',
  'downloads.panel.resumeNamed': 'Продолжить «{name}»',
  'downloads.panel.downloadNow': 'Скачать сейчас',
  'downloads.panel.downloadNowNamed': 'Скачать сейчас «{name}»',
  'downloads.panel.more': 'Ещё действия с «{name}»',
  'downloads.panel.details': 'Подробности',
  'downloads.panel.startNow': 'Запустить сейчас',
  'downloads.panel.openFolder': 'Открыть папку',
  'downloads.panel.cancel': 'Отменить загрузку',
  'downloads.panel.progressOf': 'Прогресс «{name}»',
  'downloads.panel.amount': '{done} из {total} · {percent}%',
  'downloads.panel.amountPaused': '{done} из {total} · {percent}%, на паузе',
  'downloads.panel.amountChecking': '{done} из {total} · {percent}%, проверка файлов',
  'downloads.panel.speed': 'Скорость',
  'downloads.panel.upload': 'отдача {speed}',
  'downloads.panel.eta': 'осталось {eta}',
  'downloads.panel.peers': 'пиры {peers}, сиды {seeders}',
  'downloads.panel.limit': 'лимит {limit}',
  'downloads.panel.scheduleNote': 'Сейчас вне окна расписания: загрузка продолжится, когда окно откроется.',
  'downloads.panel.cancelTitle': 'Отменить загрузку «{name}»?',
  'downloads.panel.cancelDeletes': 'Загрузка остановится, а уже скачанные {size} будут удалены с диска. Это нельзя отменить.',
  'downloads.panel.cancelKeeps': 'Загрузка остановится и уйдёт из списка. Уже записанные файлы останутся на диске.',
  'downloads.panel.cancelConfirm': 'Отменить загрузку',
  'downloads.panel.cancelBusy': 'Отмена…',
  'downloads.panel.cancelKeep': 'Не отменять',
  'downloads.panel.cancelled': 'Загрузка «{name}» отменена',
  // ---- end 6-3
} as const;

export type DownloadsKey = keyof typeof downloads;
