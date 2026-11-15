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

  // ---- Cancelling a download: one wording for the panel, the queue and the failed list (lib/confirm/prompts.ts)
  'downloads.cancel.title': 'Отменить загрузку «{name}»?',
  'downloads.cancel.deletes': 'Загрузка остановится и уйдёт из списка, а уже скачанные {size} будут удалены с диска. Это нельзя отменить.',
  'downloads.cancel.keeps': 'Загрузка остановится и уйдёт из списка. Скачанные {size} останутся на диске, в папке «{path}».',
  'downloads.cancel.busy': 'Отменяем…',
  'downloads.cancel.done': 'Загрузка «{name}» отменена',
  // ---- end of cancelling a download
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
  // ---- end 6-3
  // Stage 6-4: the queue and the failed list (routes/downloads/Queue*, FailedList, downloadCancel).
  'downloads.queue.grip': '«{title}», позиция {position} из {total}',
  'downloads.queue.gripHint': 'Перетащите, чтобы изменить порядок',
  'downloads.queue.help':
    'Стрелки вверх и вниз переносят загрузку на одно место, Home и End — в начало и в конец очереди. Escape отменяет перетаскивание.',
  'downloads.queue.position': '№ {n} в очереди',
  'downloads.queue.stateWaiting': 'Ожидает',
  'downloads.queue.stateSchedule': 'Ждёт расписания',
  'downloads.queue.stateNext': 'Начнётся следующей',
  'downloads.queue.downloaded': 'Скачано {percent}%',
  'downloads.queue.downloadedFor': 'Скачано «{title}»',
  'downloads.queue.downloadNowFor': 'Скачать сейчас: «{title}»',
  'downloads.queue.toTop': 'В начало',
  'downloads.queue.toTopFor': 'В начало очереди: «{title}»',
  'downloads.queue.details': 'Подробности',
  'downloads.queue.cancelFor': 'Отменить загрузку «{title}»',
  'downloads.queue.moved': '«{title}» перемещена: позиция {position} из {total}',
  'downloads.queue.moveFailed': 'Не удалось переместить «{title}»: {reason}',
  'downloads.failed.retryFor': 'Повторить: «{title}»',
  'downloads.failed.detailsFor': 'Подробности: «{title}»',
  'downloads.failed.downloaded': 'Скачано {done} из {total}',
  'downloads.failed.nothing': 'Ничего не скачано',
  'downloads.failed.retried': '«{title}»: новая попытка',
  // End of stage 6-4.
} as const;

export type DownloadsKey = keyof typeof downloads;
