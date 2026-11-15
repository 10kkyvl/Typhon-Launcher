export const common = {
  'common.email': 'Эл. почта',
  'common.cancel': 'Отмена',
  'common.close': 'Закрыть',
  'common.retry': 'Повторить',
  'common.save': 'Сохранить',
  'common.refresh': 'Обновить',
  'common.add': 'Добавить',
  'common.continue': 'Продолжить',
  'common.delete': 'Удалить',
  'common.open': 'Открыть',
  'common.done': 'Готово',
  'common.back': 'Назад',
  'common.copy': 'Копировать',
  'common.edit': 'Изменить',
  'common.gotIt': 'Понятно',
  'common.select': 'Выбрать',
  'common.loading': 'Загрузка',
  'common.error': 'Ошибка',
  // 7-A kit
  'common.meterAlt': '{label}: {details}',
  'common.meterPart': '{name}: {size}',
  'common.meterAtLeast': 'не меньше {size}',
  'common.meterSizeUnknown': 'размер неизвестен',
  'common.meterFree': 'Свободно',
  'common.meterFreeOf': 'Свободно {free} из {total}',
  'common.meterTotal': 'Всего {total}',
  'common.meterTotalUnknown': 'Общий объём неизвестен',
  'common.chartPeriod': 'Период',
  'common.chartValue': 'Значение',
  // /7-A kit
} as const;

export type CommonKey = keyof typeof common;
