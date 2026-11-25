import { vi } from 'vitest';

// Stand-ins of the profile screen's tests. The account service, the profile service, the
// cover upload and the chat store reach the real backend with the player's session, so each test file
// that mounts a part of this folder replaces them whole through the factories here (vi.mock is hoisted
// per file) and fakes their generated bindings, so a call that slipped past the mocks is recorded,
// not sent:
//
//   vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
//   vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
//   vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
//   vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
//   vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
//   vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
//   vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));
//
// The factories import nothing they replace: a factory that imported its own module would wait on
// itself. The players are in profileFixtures.ts.

/** Every function of lib/services/account that calls Go's account service. */
export const ACCOUNT_CALLS = [
  'bootstrapSession',
  'continueAsGuest',
  'deleteAccount',
  'fetchCurrentUser',
  'login',
  'logout',
  'openForgotPassword',
  'openStatusPage',
  'openSupportSite',
  'pickAvatar',
  'register',
  'removeAvatar',
  'updateProfile',
  'uploadAvatar',
] as const;

/** Every function of lib/services/profile that calls Go's profile service. */
export const PROFILE_CALLS = ['getProfilePreview', 'getProfileSnapshot', 'igdbIdsOf'] as const;

/** Every function of lib/services/profileCover that calls Go's account service. */
export const COVER_CALLS = ['uploadProfileCover'] as const;

/** What of the chat store reaches the messaging service. */
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

const mocks = (names: readonly string[]) => Object.fromEntries(names.map((name) => [name, vi.fn()]));

export function accountService<T extends object>(original: T) {
  return { ...original, ...mocks(ACCOUNT_CALLS) };
}

export function profileService<T extends object>(original: T) {
  return { ...original, ...mocks(PROFILE_CALLS) };
}

export function coverService<T extends object>(original: T) {
  return { ...original, ...mocks(COVER_CALLS) };
}

export function messagingStore<T extends object>(original: T) {
  return { ...original, ...mocks(MESSAGING_CALLS) };
}

/** A generated binding whose `Service` records every call instead of reaching Go. */
export async function fakeService<T extends object>(original: T, prefix: string) {
  const { fakeBinding } = await import('../../lib/testing/fakeBinding');
  return { ...original, Service: fakeBinding(prefix) };
}

export function pending<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}
