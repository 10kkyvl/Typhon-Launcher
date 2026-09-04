export const search = {
  'search.releases': { one: '{count} релиз', few: '{count} релиза', many: '{count} релизов' },
  'search.sources': { one: '{count} источник', few: '{count} источника', many: '{count} источников' },
} as const;

export type SearchKey = keyof typeof search;
