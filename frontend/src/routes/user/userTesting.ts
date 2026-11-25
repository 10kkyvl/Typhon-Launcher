import { vi } from 'vitest';
import type { PublicProfile } from '../../lib/services/social';

// Fixtures and stand-ins of the user page's tests. Each test file mocks the modules itself (vi.mock is
// hoisted per file) through the factories here, so no test reaches the social or the messaging
// backend:
//
//   vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
//   vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
//   vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
//   vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));
//
// The players here are made up.

/** Every call of Go's social service the frontend service wraps, each a mock the test answers. */
export const SOCIAL_CALLS = [
  'friends',
  'refresh',
  'kick',
  'sendRequest',
  'accept',
  'decline',
  'unfriend',
  'block',
  'unblock',
  'blocks',
  'friendCode',
  'rotateFriendCode',
  'profile',
  'profileByCode',
  'userGames',
  'userCommon',
  'gameFriends',
  'feed',
  'react',
  'unreact',
  'setNote',
] as const;

export function socialService<T extends object>(original: T) {
  return { ...original, ...Object.fromEntries(SOCIAL_CALLS.map((name) => [name, vi.fn()])) };
}

/**
 * What of the chat store reaches the messaging service: each a mock. The drafts stay real (they live
 * in localStorage), so an invitation is seen where the chat would read it.
 */
export const MESSAGING_CALLS = [
  'refreshConversations',
  'loadMessages',
  'loadMore',
  'openChat',
  'openChatById',
  'closeChat',
  'setPanelVisible',
  'markPeerRead',
  'sendMessage',
  'retryMessage',
  'editMessage',
  'toggleReaction',
  'setTyping',
  'retryMessaging',
  'initMessaging',
] as const;

export function messagingStore<T extends object>(original: T) {
  return { ...original, ...Object.fromEntries(MESSAGING_CALLS.map((name) => [name, vi.fn()])) };
}

/** A storage of its own for the chat drafts: the test runner gives the page no `localStorage`. */
export function draftStorage(): Map<string, string> {
  const drafts = new Map<string, string>();
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (key: string) => drafts.get(key) ?? null,
      setItem: (key: string, value: string) => drafts.set(key, value),
      removeItem: (key: string) => drafts.delete(key),
    },
  });
  return drafts;
}

/** A generated binding whose `Service` records every call (lib/testing/fakeBinding) instead of reaching Go. */
export async function fakeService<T extends object>(original: T, prefix: string) {
  const { fakeBinding } = await import('../../lib/testing/fakeBinding');
  return { ...original, Service: fakeBinding(prefix) };
}

export function player(patch: Partial<PublicProfile> = {}): PublicProfile {
  return {
    id: 'u1',
    username: 'ann',
    displayName: 'Ann',
    avatarUrl: '',
    card: null,
    statusEmoji: '',
    statusText: '',
    layout: null,
    autoGame: null,
    bio: '',
    relation: 'none',
    visibility: 'public',
    stats: null,
    favorites: [],
    showcase: [],
    recentlyPlayed: [],
    recentActivity: [],
    common: null,
    mutualFriends: [],
    mutualCount: 0,
    createdAt: '2026-01-01T00:00:00Z',
    presence: null,
    ...patch,
  } as PublicProfile;
}
