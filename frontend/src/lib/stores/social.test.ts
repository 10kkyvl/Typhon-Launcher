import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => vi.fn()) },
  Call: { ByID: vi.fn() },
  CancellablePromise: class {},
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/settings', () => ({
  getSettings: vi.fn(),
  saveSettings: vi.fn(),
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
  toFriendsPage: (data: unknown) => data,
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
