export const errLogs = {
  'errLogs.uploadTooLarge': 'Логи слишком большие даже после сжатия — сервер не принял архив.',
  'errLogs.uploadRateLimited': 'Слишком много отправок подряд. Подождите немного и попробуйте ещё раз.',
  'errLogs.uploadNetwork': 'Не удалось связаться с сервером. Проверьте подключение к интернету.',
  'errLogs.uploadFailed': 'Сервер отказал в приёме логов.',
  'errLogs.uploadFallback': 'Не удалось отправить логи.',
} as const;
