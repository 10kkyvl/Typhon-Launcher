import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { msg } from '../i18n';

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => vi.fn()) },
  Call: { ByID: vi.fn() },
  CancellablePromise: class {},
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('./settings', async () => {
  const { writable } = await import('svelte/store');
  return { settings: writable(null), initSettings: vi.fn(), updateSettings: vi.fn() };
});
vi.mock('../services/settings', () => ({
  getSettings: vi.fn(),
  saveSettingsPatch: vi.fn(),
  saveConsent: vi.fn(),
  setupLibrary: vi.fn(),
  proposeLibraryPath: vi.fn(),
}));
vi.mock('../services/account', () => {
  class AccountError extends Error {
    code: string;
    field: string;
    constructor(code: string, field = '') {
      super(code);
      this.name = 'AccountError';
      this.code = code;
      this.field = field;
    }
  }
  return {
    AccountError,
    bootstrapSession: vi.fn(),
    continueAsGuest: vi.fn(),
    register: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(),
    fetchCurrentUser: vi.fn(),
    updateProfile: vi.fn(),
    pickAvatar: vi.fn(),
    uploadAvatar: vi.fn(),
    removeAvatar: vi.fn(),
  };
});
vi.mock('../services/social', () => ({
  emptyFriendsPage: () => ({ friends: [], incoming: [], outgoing: [] }),
  friends: vi.fn(async () => ({ friends: [], incoming: [], outgoing: [] })),
  kick: vi.fn(async () => {}),
  refresh: vi.fn(async () => {}),
  toFriendsPage: (data: unknown) => data,
  feed: vi.fn(async () => ({ events: [], next: 0 })),
  react: vi.fn(async () => {}),
  unreact: vi.fn(async () => {}),
}));

function request(id: string) {
  return { id, username: id, displayName: id, avatarUrl: '', createdAt: '', mutualCount: 0, commonCount: 0 };
}

function friend(id: string) {
  return { id, username: id, displayName: id, avatarUrl: '', since: '' };
}

function page(friends: number, incoming: number) {
  return {
    friends: Array.from({ length: friends }, (_, i) => friend(`f${i}`)),
    incoming: Array.from({ length: incoming }, (_, i) => request(`i${i}`)),
    outgoing: [],
  };
}

async function load() {
  vi.resetModules();
  const social = await import('./social');
  const { authState } = await import('./user');
  await social.initSocial();
  authState.set('authenticated');
  return { ...social, authState };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('сброс состояния при смене аккаунта', () => {
  it('выход из аккаунта очищает список друзей', async () => {
    const { friendsPage, authState } = await load();
    friendsPage.set(page(2, 1));

    authState.set('unauthenticated');

    expect(get(friendsPage)).toEqual({ friends: [], incoming: [], outgoing: [] });
  });

  it('переход в гостя очищает список друзей', async () => {
    const { friendsPage, authState } = await load();
    friendsPage.set(page(2, 1));

    authState.set('guest');

    expect(get(friendsPage)).toEqual({ friends: [], incoming: [], outgoing: [] });
  });

  it('вход в аккаунт просит бэкенд перечитать список', async () => {
    const { authState } = await load();
    const { kick } = await import('../services/social');
    authState.set('unauthenticated');
    vi.mocked(kick).mockClear();

    authState.set('authenticated');

    expect(kick).toHaveBeenCalledTimes(1);
  });
});

describe('отключение синхронизации аккаунта', () => {
  it('снятое согласие очищает ленту', async () => {
    await load();
    const { settings } = await import('./settings');
    const { feedEvents, feedReady } = await import('./feed');
    settings.set({ accountSync: true } as never);
    feedEvents.set([{ id: 1 } as never]);
    feedReady.set(true);

    settings.set({ accountSync: false } as never);

    expect(get(feedEvents)).toEqual([]);
    expect(get(feedReady)).toBe(false);
  });

  it('включённая синхронизация ленту не трогает', async () => {
    await load();
    const { settings } = await import('./settings');
    const { feedEvents } = await import('./feed');
    settings.set({ accountSync: false } as never);
    feedEvents.set([{ id: 1 } as never]);

    settings.set({ accountSync: true } as never);

    expect(get(feedEvents)).toEqual([{ id: 1 }]);
  });
});

describe('incomingPeak', () => {
  it('уменьшение числа заявок не двигает отметку', async () => {
    const { friendsPage, incomingPeak } = await load();
    friendsPage.set(page(0, 3));
    expect(get(incomingPeak)).toBe(3);

    friendsPage.set(page(0, 2));

    expect(get(incomingPeak)).toBe(3);
  });

  it('рост выше отметки её поднимает', async () => {
    const { friendsPage, incomingPeak } = await load();
    friendsPage.set(page(0, 2));
    expect(get(incomingPeak)).toBe(2);

    friendsPage.set(page(0, 4));

    expect(get(incomingPeak)).toBe(4);
  });

  it('пустой список заявок обнуляет отметку', async () => {
    const { friendsPage, incomingPeak } = await load();
    friendsPage.set(page(0, 3));
    expect(get(incomingPeak)).toBe(3);

    friendsPage.set(page(0, 0));

    expect(get(incomingPeak)).toBe(0);
  });
});

describe('the first read of the friends', () => {
  async function signedIn() {
    const store = await load();
    const service = await import('../services/social');
    const { toast } = await import('./toasts');
    const { Events } = await import('@wailsio/runtime');
    vi.mocked(service.friends).mockReset();
    vi.mocked(service.refresh).mockReset();
    return { ...store, service, toast, Events };
  }

  it('a failure before any page is kept as the state, not said in a toast', async () => {
    const { friendsLoaded, friendsLoadError, reloadFriends, service, toast } = await signedIn();
    const failure = new Error('timeout');
    vi.mocked(service.refresh).mockRejectedValueOnce(failure);

    expect(await reloadFriends()).toBe(false);

    expect([get(friendsLoaded), get(friendsLoadError)]).toEqual([false, failure]);
    expect(toast).not.toHaveBeenCalled();
  });

  it('a page that arrives is the read: the lists, no failure', async () => {
    const { friendsLoaded, friendsLoadError, friendsPage, reloadFriends, service } = await signedIn();
    vi.mocked(service.refresh).mockRejectedValueOnce(new Error('timeout'));
    await reloadFriends();
    vi.mocked(service.friends).mockResolvedValueOnce(page(2, 0));

    expect(await reloadFriends()).toBe(true);

    expect([get(friendsLoaded), get(friendsLoadError), get(friendsPage).friends.length]).toEqual([true, null, 2]);
  });

  it('a page Go sends on its own is a read too', async () => {
    const { friendsLoaded, Events } = await signedIn();
    const handler = vi.mocked(Events.On).mock.calls.find(([name]) => name === 'social:friends')?.[1];

    handler?.({ data: page(1, 0) } as never);

    expect(get(friendsLoaded)).toBe(true);
  });

  it('a failure after a page keeps the lists and is said once', async () => {
    const { friendsLoadError, friendsPage, reloadFriends, service, toast } = await signedIn();
    vi.mocked(service.friends).mockResolvedValueOnce(page(2, 0));
    await reloadFriends();
    vi.mocked(service.refresh).mockRejectedValueOnce(new Error('timeout'));

    expect(await reloadFriends()).toBe(false);

    expect([get(friendsPage).friends.length, get(friendsLoadError)]).toEqual([2, null]);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg('social.friendsRefreshFailed'), 'danger']]);
  });

  it('an answer that comes after the account changed is dropped', async () => {
    const { authState, friendsLoaded, friendsPage, reloadFriends, service } = await signedIn();
    let answer!: (value: ReturnType<typeof page>) => void;
    vi.mocked(service.friends).mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const reading = reloadFriends();

    authState.set('unauthenticated');
    answer(page(2, 0));
    await reading;

    expect([get(friendsLoaded), get(friendsPage).friends.length]).toEqual([false, 0]);
  });

  it('a session that ends forgets the read and its failure', async () => {
    const { authState, friendsLoaded, friendsLoadError, reloadFriends, service } = await signedIn();
    vi.mocked(service.friends).mockResolvedValueOnce(page(1, 0));
    await reloadFriends();

    authState.set('guest');

    expect([get(friendsLoaded), get(friendsLoadError)]).toEqual([false, null]);
  });
});
