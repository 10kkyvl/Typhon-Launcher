export const common = {
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
} as const;

export type CommonKey = keyof typeof common;
