export const errSaves = {
  'savebackup.fallback': 'Не удалось выполнить действие с резервной копией',
  'savebackup.not_started': 'Сервис резервных копий ещё не запущен',
  'savebackup.invalid_id': 'Некорректный идентификатор игры или копии',
  'savebackup.invalid_kind': 'Неизвестный вид резервной копии',
  'savebackup.saves_not_found': 'Папка сохранений не найдена. Укажите её вручную',
  'savebackup.saves_ambiguous': 'Найдено несколько папок сохранений. Выберите нужную',
  'savebackup.saves_not_a_directory': 'Путь сохранений не является папкой',
  'savebackup.saves_path_unavailable': 'Папка сохранений недоступна',
  'savebackup.no_source': 'Не указана папка, которую нужно скопировать',
  'savebackup.game_running': 'Закройте игру перед восстановлением',
  'savebackup.snapshot_not_found': 'Резервная копия не найдена',
  'savebackup.snapshot_broken': 'Резервная копия повреждена, восстановить из неё нельзя',
  'savebackup.verify_failed': 'Копия не совпала с оригиналом. Попробуйте ещё раз',
  'savebackup.unchanged': 'Сохранения не изменились с прошлой копии',
  'savebackup.restore_leftovers':
    'Рядом с папкой сохранений остались следы прерванного восстановления. Проверьте папку и повторите',
  'savebackup.recovery_failed': 'Не удалось довести до конца прерванную операцию с сохранениями',
  'savebackup.no_free_space': 'На диске не хватает места для копии',
  'savebackup.rotation_failed': 'Копия создана, но старые копии удалить не удалось',
  'savebackup.cleanup_failed': 'не удалось убрать временные файлы рядом с папкой сохранений',
} as const;

export type ErrSavesKey = keyof typeof errSaves;
