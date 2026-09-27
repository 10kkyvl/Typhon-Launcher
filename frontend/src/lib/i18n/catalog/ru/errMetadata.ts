export const errMetadata = {
  'errMetadata.fallback': 'не удалось выполнить операцию с метаданными',
  'errMetadata.catalogGameNotFound': 'игра не найдена',
  'errMetadata.catalogNoIgdbId': 'не указан IGDB id',
  'errMetadata.catalogNoTitle': 'укажите название игры',
  'errMetadata.catalogDuplicateId': 'игра с таким идентификатором уже есть',
  'errMetadata.catalogNothingToLearn': 'нечего запоминать',
  'errMetadata.catalogNoProviderId': 'не указан идентификатор провайдера',
  'errMetadata.catalogNoTimestamp': 'не указано время обновления метаданных',
  'errMetadata.catalogSaveFailed': 'не удалось сохранить изменения в каталоге игр',
  'errMetadata.metadataNoGameId': 'не указана игра',
  'errMetadata.metadataNotStarted': 'сервис метаданных не запущен',
  'errMetadata.metadataEmptyQuery': 'пустой поисковый запрос',
  'errMetadata.metadataNoTitle': 'провайдер вернул игру без названия',
  'errMetadata.metadataBusy': 'метаданные этой игры уже обновляются',
  'errMetadata.metadataNotConfigured': 'провайдер метаданных не настроен',
  'errMetadata.metadataAmbiguousMatch': 'однозначное совпадение не найдено',
  'errMetadata.metadataNoMatch': 'игра не найдена у провайдера метаданных',
  'errMetadata.metadataRateLimited': 'сервер метаданных ограничил частоту запросов',
  'errMetadata.metadataSaveFailed': 'не удалось сохранить данные метаданных',
} as const;

export type ErrMetadataKey = keyof typeof errMetadata;
