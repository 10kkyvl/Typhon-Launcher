export const profile = {
  'profile.monthGames': { one: '{count} игра', few: '{count} игры', many: '{count} игр' },
  'profile.monthCompleted': { one: '{count} пройдена', few: '{count} пройдено', many: '{count} пройдено' },
} as const;

export type ProfileKey = keyof typeof profile;
