export const search = {
  'search.unavailable': 'Поиск недоступен',
  'search.releases': { one: '{count} релиз', few: '{count} релиза', many: '{count} релизов' },
  'search.sources': { one: '{count} источник', few: '{count} источника', many: '{count} источников' },
  'search.placeholder': 'Поиск игр и релизов',
  'search.games': 'Игры',
  'search.more': 'и ещё {count}',
  'search.releasesNoMatch': 'Релизы без совпадения',
  'search.nothingFound': 'Ничего не найдено',
} as const;

export type SearchKey = keyof typeof search;
