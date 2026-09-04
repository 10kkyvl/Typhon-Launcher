export const install = {
  'release.duplicateSources': { one: 'источник', few: 'источника', many: 'источников' },
  'verify.missingFiles': { one: '{count} файл', few: '{count} файла', many: '{count} файлов' },
  'verify.corruptedBlocks': { one: '{count} блок', few: '{count} блока', many: '{count} блоков' },
  'verify.unreadableFiles': { one: '{count} файл', few: '{count} файла', many: '{count} файлов' },
} as const;

export type InstallKey = keyof typeof install;
