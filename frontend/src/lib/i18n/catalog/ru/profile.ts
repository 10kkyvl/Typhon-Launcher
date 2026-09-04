export const profile = {
  'profile.monthGames': { one: '{count} игра', few: '{count} игры', many: '{count} игр' },
  'profile.monthCompleted': { one: '{count} пройдена', few: '{count} пройдено', many: '{count} пройдено' },
  'profile.showcaseFavorites': 'Любимые',
  'profile.showcaseRecentlyCompleted': 'Недавно пройденные',
  'profile.showcaseMostPlayed': 'Больше всего сыграно',
  'profile.showcaseHintFavorites': 'Отметьте игры сердцем на странице игры',
  'profile.showcaseHintRecentlyCompleted': 'Выберите статус «Пройдена» на странице игры',
  'profile.showcaseHintMostPlayed': 'Появится после первой сыгранной сессии',
  'profile.visibilityPublic': 'Все',
  'profile.visibilityFriends': 'Друзья',
  'profile.visibilityPrivate': 'Никто',
  'profile.today': 'Сегодня',
  'profile.yesterday': 'Вчера',
  'profile.recentWindow': '{value} за 2 недели',
} as const;

export type ProfileKey = keyof typeof profile;
