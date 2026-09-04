export const friends = {
  'friends.requestOne': 'Заявка в друзья',
  'friends.requests': {
    one: '{count} заявка в друзья',
    few: '{count} заявки в друзья',
    many: '{count} заявок в друзья',
  },
  'friends.commonFriends': {
    one: '{count} общий друг',
    few: '{count} общих друга',
    many: '{count} общих друзей',
  },
  'friends.commonGames': {
    one: '{count} общая игра',
    few: '{count} общие игры',
    many: '{count} общих игр',
  },
  'friends.played': {
    one: '{count} друг играл',
    few: '{count} друга играли',
    many: '{count} друзей играли',
  },
  'friends.sentAt': 'Отправлена {when}',
  'friends.receivedAt': 'Получена {when}',
  'friends.pendingReply': 'Ожидает ответа',
} as const;

export type FriendsKey = keyof typeof friends;
