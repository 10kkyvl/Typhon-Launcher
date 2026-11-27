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

/** A launcher started signed out, and then a sign-in: nothing has been read for the account yet. */
async function load() {
  vi.resetModules();
  const social = await import('./social');
  const { authState } = await import('./user');
  await social.initSocial();
  authState.set('unauthenticated');
  authState.set('authenticated');
  return { ...social, authState };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

async function flush() {
  for (let i = 0; i < 10; i++) await Promise.resolve();
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

  it('reads asked while one runs make one more read after it, which all of them wait for', async () => {
    const { friendsPage, reloadFriends, service } = await signedIn();
    let first!: () => void;
    vi.mocked(service.refresh).mockReturnValueOnce(new Promise<void>((resolve) => (first = resolve)));
    vi.mocked(service.friends).mockResolvedValueOnce(page(1, 0)).mockResolvedValueOnce(page(3, 0));

    const running = reloadFriends();
    const asked = [reloadFriends(), reloadFriends(), reloadFriends()];
    await Promise.resolve();
    expect(service.refresh).toHaveBeenCalledTimes(1);

    first();
    expect(await running).toBe(true);
    expect(await Promise.all(asked)).toEqual([true, true, true]);

    expect([vi.mocked(service.refresh).mock.calls.length, vi.mocked(service.friends).mock.calls.length]).toEqual([2, 2]);
    expect(get(friendsPage).friends.length).toBe(3);
  });

  it('a read of an ended session does not hold the next session’s', async () => {
    const { authState, friendsLoaded, reloadFriends, service } = await signedIn();
    vi.mocked(service.refresh).mockReturnValueOnce(new Promise<void>(() => {}));
    void reloadFriends();

    authState.set('unauthenticated');
    authState.set('authenticated');
    vi.mocked(service.friends).mockResolvedValueOnce(page(2, 0));

    expect(await reloadFriends()).toBe(true);
    expect(get(friendsLoaded)).toBe(true);
  });

  it('a page Go sent while the read waited for Friends() is not replaced by the older answer', async () => {
    const { friendsPage, reloadFriends, service, Events } = await signedIn();
    const handler = vi.mocked(Events.On).mock.calls.find(([name]) => name === 'social:friends')?.[1] as (event: { data: unknown }) => void;
    let answer!: (value: ReturnType<typeof page>) => void;
    vi.mocked(service.friends).mockReturnValueOnce(new Promise((resolve) => (answer = resolve)) as never);
    const reading = reloadFriends();
    await vi.waitFor(() => expect(service.friends).toHaveBeenCalled());

    handler({ data: page(3, 0) });
    answer(page(2, 0));

    expect(await reading).toBe(true);
    expect(get(friendsPage).friends.length).toBe(3);
  });

  it('a page Go sent before the read asked for Friends() does not stop its answer', async () => {
    const { friendsPage, reloadFriends, service, Events } = await signedIn();
    const handler = vi.mocked(Events.On).mock.calls.find(([name]) => name === 'social:friends')?.[1] as (event: { data: unknown }) => void;
    let refreshed!: () => void;
    vi.mocked(service.refresh).mockReturnValueOnce(new Promise<void>((resolve) => (refreshed = resolve)));
    vi.mocked(service.friends).mockResolvedValueOnce(page(4, 0));
    const reading = reloadFriends();

    handler({ data: page(3, 0) });
    refreshed();

    expect(await reading).toBe(true);
    expect(get(friendsPage).friends.length).toBe(4);
  });

  it('a session that ends forgets the read and its failure', async () => {
    const { authState, friendsLoaded, friendsLoadError, reloadFriends, service } = await signedIn();
    vi.mocked(service.friends).mockResolvedValueOnce(page(1, 0));
    await reloadFriends();

    authState.set('guest');

    expect([get(friendsLoaded), get(friendsLoadError)]).toEqual([false, null]);
  });
});

describe('the read after the sync consent (loadFriends)', () => {
  async function signedIn() {
    const store = await load();
    const service = await import('../services/social');
    const { toast } = await import('./toasts');
    vi.mocked(service.friends).mockReset();
    vi.mocked(toast).mockClear();
    return { ...store, service, toast };
  }

  it('that fails before any page is kept as the state and said once in a toast', async () => {
    const { friendsLoaded, friendsLoadError, loadFriends, service, toast } = await signedIn();
    const failure = new Error('timeout');
    vi.mocked(service.friends).mockRejectedValueOnce(failure);

    await loadFriends();

    expect([get(friendsLoaded), get(friendsLoadError)]).toEqual([false, failure]);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg('state.socialLoadFriendsFailed'), 'danger']]);
  });

  it('that fails while a screen shows the state itself is kept for it, without a toast', async () => {
    const { friendsLoadError, loadFriends, service, showFriendsState, toast } = await signedIn();
    const failure = new Error('timeout');
    vi.mocked(service.friends).mockRejectedValueOnce(failure).mockRejectedValueOnce(failure);
    const release = showFriendsState();

    await loadFriends();
    expect(get(friendsLoadError)).toBe(failure);
    expect(toast).not.toHaveBeenCalled();

    release();
    await loadFriends();
    expect(vi.mocked(toast).mock.calls).toEqual([[msg('state.socialLoadFriendsFailed'), 'danger']]);
  });

  it('that fails after a page keeps the lists and says it once, whoever shows the state', async () => {
    const { friendsPage, friendsLoadError, loadFriends, service, showFriendsState, toast } = await signedIn();
    vi.mocked(service.friends).mockResolvedValueOnce(page(2, 0)).mockRejectedValueOnce(new Error('timeout'));
    await loadFriends();
    const release = showFriendsState();

    await loadFriends();
    release();

    expect([get(friendsPage).friends.length, get(friendsLoadError)]).toEqual([2, null]);
    expect(vi.mocked(toast).mock.calls).toEqual([[msg('state.socialLoadFriendsFailed'), 'danger']]);
  });

  it('is not said when the session is gone', async () => {
    const { AccountError } = await import('../services/account');
    const { friendsLoadError, loadFriends, service, toast } = await signedIn();
    vi.mocked(service.friends).mockRejectedValueOnce(new AccountError('unauthenticated'));

    await loadFriends();

    expect(get(friendsLoadError)).toBeInstanceOf(AccountError);
    expect(toast).not.toHaveBeenCalled();
  });
});

describe('the read of a start (the stored session checked behind the splash)', () => {
  /** A launcher that starts: the store is started while the stored session is checked. */
  async function starting() {
    vi.resetModules();
    const service = await import('../services/social');
    const { toast } = await import('./toasts');
    const { Events } = await import('@wailsio/runtime');
    vi.mocked(service.friends).mockReset();
    vi.mocked(service.refresh).mockReset();
    const social = await import('./social');
    const { authState } = await import('./user');
    const handler = () =>
      vi.mocked(Events.On).mock.calls.find(([name]) => name === 'social:friends')?.[1] as (event: { data: unknown }) => void;
    return { ...social, authState, service, toast, handler };
  }

  const held = (store: Awaited<ReturnType<typeof starting>>) => ({
    friends: get(store.friendsPage).friends.length,
    incoming: get(store.incomingCount),
    loaded: get(store.friendsLoaded),
    error: get(store.friendsLoadError),
  });
  const asked = (service: Awaited<ReturnType<typeof starting>>['service']) =>
    [service.kick, service.refresh, service.friends].map((call) => vi.mocked(call).mock.calls.length);

  it('S5 a signed-in start has the page the moment the splash goes: one Friends(), one kick, no refresh', async () => {
    const store = await starting();
    vi.mocked(store.service.friends).mockResolvedValue(page(3, 1) as never);

    await store.initSocial();
    store.authState.set('authenticated');
    await flush();

    expect(held(store)).toEqual({ friends: 3, incoming: 1, loaded: true, error: null });
    expect(asked(store.service)).toEqual([1, 0, 1]);
    expect(store.toast).not.toHaveBeenCalled();
  });

  it('an answer that comes after the stored session was confirmed is the page of that account', async () => {
    const store = await starting();
    const answer = deferred<ReturnType<typeof page>>();
    vi.mocked(store.service.friends).mockReturnValueOnce(answer.promise as never);

    const started = store.initSocial();
    store.authState.set('authenticated');
    answer.resolve(page(2, 1));
    await started;

    expect(held(store)).toEqual({ friends: 2, incoming: 1, loaded: true, error: null });
  });

  it('that fails is the state only, never a toast; a failure before the sign-in settled is forgotten then', async () => {
    const store = await starting();
    const failure = new Error('timeout');
    vi.mocked(store.service.friends).mockRejectedValueOnce(failure);

    await store.initSocial();
    expect(held(store)).toEqual({ friends: 0, incoming: 0, loaded: false, error: failure });

    store.authState.set('authenticated');

    expect(held(store)).toEqual({ friends: 0, incoming: 0, loaded: false, error: null });
    expect(store.toast).not.toHaveBeenCalled();
  });

  it('that fails once signed in is the state only, never a toast', async () => {
    const store = await starting();
    const failure = new Error('timeout');
    const answer = deferred<ReturnType<typeof page>>();
    vi.mocked(store.service.friends).mockReturnValueOnce(answer.promise as never);

    const started = store.initSocial();
    store.authState.set('authenticated');
    answer.reject(failure);
    await started;

    expect(held(store)).toEqual({ friends: 0, incoming: 0, loaded: false, error: failure });
    expect(store.toast).not.toHaveBeenCalled();
  });

  it('gives way to a newer page Go sent while it ran', async () => {
    const store = await starting();
    const answer = deferred<ReturnType<typeof page>>();
    vi.mocked(store.service.friends).mockReturnValueOnce(answer.promise as never);

    const started = store.initSocial();
    store.authState.set('authenticated');
    store.handler()({ data: page(1, 0) });
    answer.resolve(page(4, 2));
    await started;

    expect(held(store)).toEqual({ friends: 1, incoming: 0, loaded: true, error: null });
  });

  it('is the Friends screen’s read when the screen arrives while it runs: no second read', async () => {
    const store = await starting();
    const answer = deferred<ReturnType<typeof page>>();
    vi.mocked(store.service.friends).mockReturnValueOnce(answer.promise as never);

    const started = store.initSocial();
    store.authState.set('authenticated');
    const arrival = store.readFriendsOnArrival();
    answer.resolve(page(2, 0));

    expect(await arrival).toBe(true);
    await started;
    expect(asked(store.service)).toEqual([1, 0, 1]);
    expect(held(store).friends).toBe(2);
  });

  it('is not the read of a screen that arrives after it answered: that one reads as on any arrival', async () => {
    const store = await starting();
    vi.mocked(store.service.friends).mockResolvedValueOnce(page(2, 0) as never).mockResolvedValueOnce(page(3, 0) as never);

    await store.initSocial();
    store.authState.set('authenticated');

    expect(await store.readFriendsOnArrival()).toBe(true);
    expect(asked(store.service)).toEqual([1, 1, 2]);
    expect(held(store).friends).toBe(3);
  });

  it('does not stand for a read asked after an action: that one waits for a read that starts after it', async () => {
    const store = await starting();
    const answer = deferred<ReturnType<typeof page>>();
    vi.mocked(store.service.friends).mockReturnValueOnce(answer.promise as never).mockResolvedValueOnce(page(5, 0) as never);

    const started = store.initSocial();
    store.authState.set('authenticated');
    const afterAction = store.reloadFriends();
    answer.resolve(page(2, 0));

    expect(await afterAction).toBe(true);
    await started;
    expect(asked(store.service)).toEqual([1, 1, 2]);
    expect(held(store).friends).toBe(5);
  });
});
