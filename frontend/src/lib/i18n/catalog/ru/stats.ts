export const stats = {
  'stats.nav': 'Статистика',
  'stats.title': 'Статистика',
  'stats.subtitle': 'Сколько вы играли за выбранный период',
  'stats.periodLabel': 'Период',
  'stats.periodWeek': 'Неделя',
  'stats.periodMonth': 'Месяц',
  'stats.periodYear': 'Год',
  'stats.totalLabel': 'Всего наиграно',
  'stats.sessionsLabel': 'Сессий',
  'stats.longestLabel': 'Самая длинная сессия',
  'stats.longestSub': '{game} · {date}',
  'stats.longestNone': 'Нет данных',
  'stats.chartTitle': 'По дням',
  'stats.chartTitleYear': 'По месяцам',
  'stats.chartIdle': 'не играли',
  'stats.topTitle': 'Больше всего времени',
  'stats.topSessions': { one: '{count} сессия', few: '{count} сессии', many: '{count} сессий', other: '{count} сессии' },
  'stats.deletedGame': 'Удалённая игра',
  'stats.empty': 'За этот период игр не было. Запустите игру, и здесь появится статистика.',
  'stats.loadError': 'Не удалось загрузить статистику',
  'stats.loadErrorReason': 'Не удалось загрузить статистику: {reason}',
  'stats.retry': 'Повторить',
  'stats.loading':'Считаем статистику',
  // 7-B stats
  'stats.topLine': { one: '{time} за {count} сессию', few: '{time} за {count} сессии', many: '{time} за {count} сессий', other: '{time} за {count} сессии' },
  'stats.topShare': '{share}% всего времени за период',
  'stats.chartDay': 'День',
  'stats.chartMonth': 'Месяц',
  'stats.chartPlaytime': 'Время в игре',
  // /7-B stats
} as const;

export type StatsKey = keyof typeof stats;
