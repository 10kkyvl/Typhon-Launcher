export const errUpdates = {
  'errUpdates.updatesNotTracked': 'для этой игры нет данных об обновлении',
  'errUpdates.updatesNoInstallDir': 'каталог установки не задан',
  'errUpdates.updatesNoPlan': 'сначала подготовьте план обновления',
  'errUpdates.updatesGameRunning': 'игра запущена — закройте её перед обновлением',
  'errUpdates.updatesBusy': 'операция уже выполняется',
  'errUpdates.updatesNoRollback': 'предыдущая версия недоступна',
  'errUpdates.updatesNoIdentity': 'проверка недоступна для этой установки',
  'errUpdates.updatesNoDownloads': 'менеджер загрузок недоступен',
  'errUpdates.updatesNoInstaller': 'установщик недоступен',
  'errUpdates.updatesNoLibrary': 'библиотека недоступна',
  'errUpdates.updatesUpdateFailed': 'не удалось применить обновление',
  'errUpdates.updatesDownloadFailed': 'не удалось скачать данные обновления',
  'errUpdates.updatesInstallFailed': 'не удалось установить обновление',
  'errUpdates.updatesStagingEmpty': 'временная установка пуста',
  'errUpdates.updatesNoLaunchTarget': 'исполняемый файл не найден после обновления',
  'errUpdates.updatesSwapFailed': 'не удалось заменить установленную версию',
  'errUpdates.updatesCarryOverFailed': 'не удалось перенести пользовательские файлы из предыдущей версии',
  'errUpdates.updatesPrefetchUnavailable': 'предварительная загрузка недоступна для этой стратегии',
  'errUpdates.updatesNoFreeSpaceForBackup': 'недостаточно места для резервной копии перед обновлением',
  'errUpdates.updatesDownloadStalled': 'загрузка остановилась: нет сети или источников, повторите обновление позже',
  'errUpdates.updatesNoTarget': 'релиз для обновления недоступен',
  'errUpdates.updatesRepairUnavailable': 'восстановление недоступно для этой установки',
  'errUpdates.selfupdateReadOnly':
    'не удалось сохранить данные обновления из-за более ранней ошибки чтения. Перезапустите Typhon',
  'errUpdates.selfupdateManifestOutdated':
    'эта версия Typhon больше не поддерживается сервером обновлений. Установите новую версию вручную',
} as const;

export type ErrUpdatesKey = keyof typeof errUpdates;
