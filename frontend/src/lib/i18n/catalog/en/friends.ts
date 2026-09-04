import type { Message } from '../../types';
import type { FriendsKey } from '../ru/friends';

export const friends: Record<FriendsKey, Message> = {
  'friends.requestOne': 'Friend request',
  'friends.requests': { one: '{count} friend request', other: '{count} friend requests' },
  'friends.commonFriends': { one: '{count} mutual friend', other: '{count} mutual friends' },
  'friends.commonGames': { one: '{count} game in common', other: '{count} games in common' },
  'friends.played': { one: '{count} friend played', other: '{count} friends played' },
};
