export const errLogs = {
  'errLogs.uploadTimeout': 'Сервер не успел ответить на отправку логов. Попробуйте позже.',
  'errLogs.uploadCancelled': 'Отправка логов отменена.',
  'errLogs.uploadTooLarge': 'Логи слишком большие даже после сжатия — сервер не принял архив.',
  'errLogs.uploadRateLimited': 'Слишком много отправок подряд. Подождите немного и попробуйте ещё раз.',
  'errLogs.uploadNetwork': 'Не удалось связаться с сервером. Проверьте подключение к интернету.',
  'errLogs.uploadUnavailable': 'Сервер сейчас не принимает логи. Попробуйте позже.',
  'errLogs.uploadRejected': 'Сервер не принял этот архив логов.',
  'errLogs.uploadFailed': 'Сервер отказал в приёме логов.',
  'errLogs.uploadBusy': 'Отправка логов уже выполняется.',
  'errLogs.uploadFallback': 'Не удалось отправить логи.',
} as const;
