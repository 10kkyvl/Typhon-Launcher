import { vi } from 'vitest';
import type { FriendView, FriendsPage, RequestView, UserCard } from '../../lib/services/social';

// Fixtures and stand-ins of the friends screen's dom tests. Each test file mocks the modules itself
// (vi.mock is hoisted per file) through the factories here, so no test reaches the social or the
// messaging backend (Friends.mocks.test.ts holds every file of the folder to it):
//
//   vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
//   vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
//   vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
//   vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));
//
// The people here are made up.

const unanswered = (name: string) => async () => {
  throw new Error(`${name} is not answered by this test`);
};

/** Every call of Go's social service (lib/services/social), each a mock the test answers. */
export function socialService<T extends object>(original: T) {
  return {
    ...original,
    friends: vi.fn(async (): Promise<FriendsPage> => ({ friends: [], incoming: [], outgoing: [] })),
    refresh: vi.fn(async () => {}),
    kick: vi.fn(async () => {}),
    sendRequest: vi.fn(unanswered('sendRequest')),
    accept: vi.fn(async () => {}),
    decline: vi.fn(async () => {}),
    unfriend: vi.fn(async () => {}),
    block: vi.fn(async () => {}),
    unblock: vi.fn(async () => {}),
    blocks: vi.fn(async (): Promise<UserCard[]> => []),
    friendCode: vi.fn(async () => FRIEND_CODE),
    rotateFriendCode: vi.fn(async () => NEW_FRIEND_CODE),
    profile: vi.fn(unanswered('profile')),
    profileByCode: vi.fn(unanswered('profileByCode')),
    userGames: vi.fn(async () => ({ games: [], next: '' })),
    userCommon: vi.fn(async () => ({ games: [], next: '' })),
    gameFriends: vi.fn(async () => ({ friends: [], total: 0 })),
    feed: vi.fn(async () => ({ events: [], next: '' })),
    react: vi.fn(async () => {}),
    unreact: vi.fn(async () => {}),
    setNote: vi.fn(async () => {}),
  };
}

/** Every call of Go's messaging service (lib/services/messaging): the chat a row's button opens reads through it. */
export function messagingService<T extends object>(original: T) {
  return {
    ...original,
    conversations: vi.fn(async () => []),
    messages: vi.fn(async () => ({ messages: [], next: '', canSend: true })),
    send: vi.fn(unanswered('send')),
    pickChatImages: vi.fn(async () => []),
    uploadChatImage: vi.fn(unanswered('uploadChatImage')),
    edit: vi.fn(unanswered('edit')),
    react: vi.fn(async () => {}),
    unreact: vi.fn(async () => {}),
    read: vi.fn(async () => {}),
    typing: vi.fn(async () => {}),
    start: vi.fn(async () => {}),
    stop: vi.fn(async () => {}),
    notify: vi.fn(async () => false),
  };
}

/**
 * A generated binding whose `Service` records every call instead of reaching Go: the tests assert that
 * `calls` (lib/testing/fakeBinding) stays empty, so a call that slipped past the mocks above is seen,
 * not sent.
 */
export async function fakeService<T extends object>(original: T, prefix: string) {
  const { fakeBinding } = await import('../../lib/testing/fakeBinding');
  return { ...original, Service: fakeBinding(prefix) };
}

export const FRIEND_CODE = 'TY-7K2M-9QXW';
export const NEW_FRIEND_CODE = 'TY-4HJN-8RPT';

export const card = (id: string, name: string, extra: Partial<UserCard> = {}): UserCard => ({
  id,
  username: name.toLowerCase(),
  displayName: name,
  avatarUrl: '',
  ...extra,
});

export const friend = (id: string, name: string, status: string, extra: Partial<FriendView> = {}): FriendView => ({
  ...card(id, name),
  since: '2026-01-01T00:00:00Z',
  presence: { status },
  ...extra,
});

export const request = (id: string, name: string, commonCount = 0): RequestView => ({
  ...card(id, name),
  createdAt: '2026-11-20T11:00:00Z',
  mutualCount: 0,
  commonCount,
});

/** jsdom has no ResizeObserver; the page's segmented controls measure with one. */
export class NoResize {
  observe() {}
  unobserve() {}
  disconnect() {}
}
