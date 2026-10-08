import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get, writable } from 'svelte/store';
import type { FriendsPage } from '../lib/services/social';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

vi.mock('../lib/services/backend', () => ({ inWails: true }));

const fetchCurrentUser = vi.fn();
const fetchFriends = vi.fn();
const refreshConversations = vi.fn(async () => {});
const loadMessages = vi.fn(async () => {});
const activePeer = writable<{ id: string } | null>(null);

vi.mock('../lib/services/account', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  fetchCurrentUser,
}));

vi.mock('../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  friends: fetchFriends,
}));

vi.mock('../lib/stores/messaging', () => ({
  activePeer,
  loadMessages,
  refreshConversations,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const data = await import('./data');
  const account = await import('../lib/services/account');
  const stores = await import('../lib/stores/user');
  const social = await import('../lib/stores/social');
  const socialService = await import('../lib/services/social');
  return { data, account, stores, social, socialService };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(console, 'warn').mockImplementation(() => {});
  activePeer.set(null);
  fetchCurrentUser.mockResolvedValue({ id: 'u1', username: 'me' });
  fetchFriends.mockResolvedValue({ friends: [], incoming: [], outgoing: [] });
  refreshConversations.mockResolvedValue(undefined);
  loadMessages.mockResolvedValue(undefined);
});

describe('overlay account', () => {
  it('starts out loading', async () => {
    const { data } = await load();

    expect(get(data.overlayAccount)).toBe('loading');
    expect(get(data.overlayFriendsFailed)).toBe(false);
  });

  it('becomes ready, signs the user in and loads friends and conversations', async () => {
    const { data, stores } = await load();

    await data.refreshOverlay();

    expect(get(data.overlayAccount)).toBe('ready');
    expect(get(stores.authState)).toBe('authenticated');
    expect(get(stores.currentUser)).toMatchObject({ id: 'u1' });
    expect(fetchFriends).toHaveBeenCalledTimes(1);
    expect(refreshConversations).toHaveBeenCalledTimes(1);
  });

  it('shows the signed-out state and loads nothing else when the player is not signed in', async () => {
    const { data, stores, account } = await load();
    fetchCurrentUser.mockRejectedValue(new account.AccountError('unauthenticated'));

    await data.refreshOverlay();

    expect(get(data.overlayAccount)).toBe('signedOut');
    expect(get(stores.authState)).toBe('unauthenticated');
    expect(get(stores.currentUser)).toBeNull();
    expect(fetchFriends).not.toHaveBeenCalled();
    expect(refreshConversations).not.toHaveBeenCalled();
  });

  it('forgets friends of a previous account when the player signed out', async () => {
    const { data, account, social } = await load();
    social.friendsPage.set({ friends: [{ id: 'f1' }], incoming: [], outgoing: [] } as unknown as FriendsPage);
    fetchCurrentUser.mockRejectedValue(new account.AccountError('unauthenticated'));

    await data.refreshOverlay();

    expect(get(social.friendsPage).friends).toEqual([]);
  });

  it('shows an error state, not a sign-in prompt, when the account cannot be read for another reason', async () => {
    const { data, account } = await load();
    fetchCurrentUser.mockRejectedValue(new account.AccountError('network_error'));

    await data.refreshOverlay();

    expect(get(data.overlayAccount)).toBe('error');
    expect(fetchFriends).not.toHaveBeenCalled();
  });

  it('treats a failure that is not an account error as a failure too', async () => {
    const { data } = await load();
    fetchCurrentUser.mockRejectedValue(new Error('bridge down'));

    await data.refreshOverlay();

    expect(get(data.overlayAccount)).toBe('error');
  });
});

describe('overlay friends', () => {
  it('flags a failed friends list without dropping the account state', async () => {
    const { data } = await load();
    fetchFriends.mockRejectedValue(new Error('offline'));

    await data.refreshOverlay();

    expect(get(data.overlayAccount)).toBe('ready');
    expect(get(data.overlayFriendsFailed)).toBe(true);
  });

  it('clears the flag when a retry succeeds', async () => {
    const { data } = await load();
    fetchFriends.mockRejectedValueOnce(new Error('offline'));
    await data.refreshOverlay();

    await data.retryFriends();

    expect(get(data.overlayFriendsFailed)).toBe(false);
  });

  it('keeps the flag when the retry fails again', async () => {
    const { data } = await load();
    fetchFriends.mockRejectedValue(new Error('offline'));
    await data.refreshOverlay();

    await data.retryFriends();

    expect(get(data.overlayFriendsFailed)).toBe(true);
  });

  it('opens the conversation the player had open, without letting its failure break the refresh', async () => {
    const { data } = await load();
    activePeer.set({ id: 'peer-1' });
    loadMessages.mockRejectedValue(new Error('history unavailable'));

    await expect(data.refreshOverlay()).resolves.toBeUndefined();

    expect(loadMessages).toHaveBeenCalledWith('peer-1', '', false, true);
    expect(get(data.overlayAccount)).toBe('ready');
  });

  it('applies a friends list pushed by the backend and clears the failure flag', async () => {
    const { data, social } = await load();
    fetchFriends.mockRejectedValue(new Error('offline'));
    await data.refreshOverlay();
    expect(get(data.overlayFriendsFailed)).toBe(true);

    data.listenFriends();
    handlers['social:friends']({ data: { friends: [{ id: 'f1', username: 'x', displayName: 'X', avatarUrl: '' }], incoming: [], outgoing: [] } });

    expect(get(data.overlayFriendsFailed)).toBe(false);
    expect(get(social.friendsPage).friends).toHaveLength(1);
  });
});

describe('refreshing the overlay', () => {
  it('shares one refresh between callers that arrive while it is running', async () => {
    const { data } = await load();
    const slow = deferred<{ id: string }>();
    fetchCurrentUser.mockReturnValueOnce(slow.promise);

    const first = data.refreshOverlay();
    const second = data.refreshOverlay();
    expect(second).toBe(first);

    slow.resolve({ id: 'u1' });
    await first;
    expect(fetchCurrentUser).toHaveBeenCalledTimes(1);
  });

  it('starts a new refresh once the previous one is over', async () => {
    const { data } = await load();

    await data.refreshOverlay();
    await data.refreshOverlay();

    expect(fetchCurrentUser).toHaveBeenCalledTimes(2);
  });

  it('starts a new refresh after a failed one', async () => {
    const { data } = await load();
    fetchCurrentUser.mockRejectedValueOnce(new Error('bridge down'));

    await data.refreshOverlay();
    await data.refreshOverlay();

    expect(get(data.overlayAccount)).toBe('ready');
  });
});
