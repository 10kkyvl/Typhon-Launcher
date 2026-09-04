export const install = {
  'release.duplicateSources': { one: 'источник', few: 'источника', many: 'источников' },
  'release.loading': 'Загрузка релизов…',
  'release.new': 'Новое',
  'release.unavailable': 'Недоступно',
  'verify.missingFiles': { one: '{count} файл', few: '{count} файла', many: '{count} файлов' },
  'verify.corruptedBlocks': { one: '{count} блок', few: '{count} блока', many: '{count} блоков' },
  'verify.unreadableFiles': { one: '{count} файл', few: '{count} файла', many: '{count} файлов' },
  'verify.fileIntegrity': 'Целостность файлов',
  'verify.unavailable': 'Проверка недоступна',
  'verify.neverChecked': 'Не проверялось',
  'verify.damageFound': 'Найдены повреждения',
  'verify.notFullyChecked': 'Проверено не всё',
  'verify.filesOk': 'Файлы в порядке',
  'verify.unavailableExplain':
    'Сверять не с чем: релиз не даёт пригодного для проверки состава файлов, а манифест ещё не записан. Запишите манифест текущих файлов — последующие проверки покажут, что изменилось.',
  'verify.createManifest': 'Создать манифест',
  'verify.gameRunningManifest': 'Игра запущена. Закройте её, чтобы записать манифест.',
  'verify.matched': 'Совпало',
  'verify.checked': 'Проверено',
  'verify.missing': 'Отсутствуют',
  'verify.corrupted': 'Повреждены',
  'verify.unreadable': 'Не прочитаны',
  'verify.byTorrent': 'по торренту релиза',
  'verify.byManifest': 'по сохранённому манифесту',
  'verify.summary': 'Сверка {method}, {date}.',
  'verify.someFilesLocked':
    'Часть файлов была занята другой программой — это не повреждение, повторите проверку позже.',
  'verify.explain':
    'Проверка сверяет файлы на диске с торрентом релиза или с сохранённым манифестом. Результат относится к текущей версии и каталогу установки.',
  'verify.verifyFiles': 'Проверить файлы',
  'verify.repair': 'Восстановить',
  'verify.gameRunningVerify': 'Игра запущена. Закройте её перед проверкой файлов.',
} as const;

export type InstallKey = keyof typeof install;
