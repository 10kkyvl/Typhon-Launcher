export const search = {
  'search.unavailable': 'Поиск недоступен',
  'search.releases': { one: '{count} релиз', few: '{count} релиза', many: '{count} релизов' },
  'search.sources': { one: '{count} источник', few: '{count} источника', many: '{count} источников' },
  'search.placeholder': 'Поиск игр и релизов',
  'search.games': 'Игры',
  'search.more': 'и ещё {count}',
  'search.moreInCatalog': 'Ещё {count} в каталоге',
  'search.releasesNoMatch': 'Релизы без совпадения',
  'search.nothingFound': 'Ничего не найдено',
  'search.results': 'Результаты поиска',
  'search.searching': 'Идёт поиск…',
  'search.found': { one: 'Найден {count} результат', few: 'Найдено {count} результата', many: 'Найдено {count} результатов' },
} as const;

export type SearchKey = keyof typeof search;
