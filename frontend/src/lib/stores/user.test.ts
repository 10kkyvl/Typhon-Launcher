import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import type { ProfileSettings } from '../services/account';

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

const DEFAULT_PROFILE: ProfileSettings = {
  showStats: true,
  showPlaying: true,
  showActivity: true,
  showOnline: true,
  showcase: ['favorites'],
};

function makeUser(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: 'u1',
    username: 'egor',
    displayName: 'Egor',
    email: 'egor@example.com',
    avatarUrl: '',
    profile: DEFAULT_PROFILE,
    createdAt: '2024-01-01T00:00:00Z',
    ...overrides,
  };
}

function emptyUser() {
  return { id: '', username: '', displayName: '', email: '', avatarUrl: '', profile: DEFAULT_PROFILE, createdAt: '' };
}

async function loadModules() {
  vi.resetModules();
  const accountMock = await import('../services/account');
  const userStore = await import('./user');
  const router = await import('./router');
  return { accountMock, userStore, router };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('initAuth', () => {
  it('starts in the bootstrapping state so no auth screen flashes', async () => {
    const { accountMock, userStore } = await loadModules();
    let resolve!: (value: unknown) => void;
    vi.mocked(accountMock.bootstrapSession).mockReturnValue(
      new Promise((r) => {
        resolve = r as (value: unknown) => void;
      }) as never,
    );

    expect(get(userStore.authState)).toBe('bootstrapping');

    const pending = userStore.initAuth();
    expect(get(userStore.authState)).toBe('bootstrapping');

    resolve({ status: 'unauthenticated', user: emptyUser(), reason: '' });
    await pending;

    expect(get(userStore.authState)).toBe('unauthenticated');
  });

  it('authenticates from a restored session', async () => {
    const { accountMock, userStore } = await loadModules();
    const user = makeUser();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue({
      status: 'authenticated',
      user,
      reason: '',
    } as never);

    await userStore.initAuth();

    expect(get(userStore.authState)).toBe('authenticated');
    expect(get(userStore.currentUser)).toEqual(user);
  });

  it('bootstraps only once when called twice', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue({
      status: 'unauthenticated',
      user: emptyUser(),
      reason: '',
    } as never);

    await Promise.all([userStore.initAuth(), userStore.initAuth()]);

    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(1);
  });

  it('keeps the user out of the app but does not authenticate when the backend is unreachable', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue({
      status: 'unavailable',
      user: emptyUser(),
      reason: 'network_error',
    } as never);

    await userStore.initAuth();

    expect(get(userStore.authState)).toBe('unavailable');
    expect(get(userStore.authReason)).toBe('network_error');
    expect(get(userStore.currentUser)).toBeNull();
  });

  it('treats a thrown error as unavailable rather than as a logout', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockRejectedValue(new accountMock.AccountError('network_error'));

    await userStore.initAuth();

    expect(get(userStore.authState)).toBe('unavailable');
    expect(get(userStore.authReason)).toBe('network_error');
  });

  it('retryBootstrap runs another bootstrap after a failure', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValueOnce({
      status: 'unavailable',
      user: emptyUser(),
      reason: 'network_error',
    } as never);
    await userStore.initAuth();

    const user = makeUser();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValueOnce({
      status: 'authenticated',
      user,
      reason: '',
    } as never);
    await userStore.retryBootstrap();

    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(2);
    expect(get(userStore.authState)).toBe('authenticated');
  });
});

describe('guest mode', () => {
  it('enters the app as a guest without touching the profile', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.continueAsGuest).mockResolvedValue(undefined);

    await userStore.enterAsGuest();

    expect(get(userStore.authState)).toBe('guest');
    expect(get(userStore.currentUser)).toBeNull();
    expect(accountMock.fetchCurrentUser).not.toHaveBeenCalled();
  });

  it('restores guest mode on the next bootstrap', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue({
      status: 'guest',
      user: emptyUser(),
      reason: '',
    } as never);

    await userStore.initAuth();

    expect(get(userStore.authState)).toBe('guest');
    expect(get(userStore.currentUser)).toBeNull();
  });

  it('leaving guest mode clears it through the backend and opens the chosen view', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.continueAsGuest).mockResolvedValue(undefined);
    await userStore.enterAsGuest();

    vi.mocked(accountMock.logout).mockResolvedValue(undefined);
    await userStore.leaveGuest('register');

    expect(accountMock.logout).toHaveBeenCalledTimes(1);
    expect(get(userStore.authState)).toBe('unauthenticated');
    expect(get(userStore.authView)).toBe('register');
  });

  it('keeps guest mode when the marker cannot be written', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue({
      status: 'unauthenticated',
      user: emptyUser(),
      reason: '',
    } as never);
    await userStore.initAuth();

    vi.mocked(accountMock.continueAsGuest).mockRejectedValue(new accountMock.AccountError('server_error'));

    await expect(userStore.enterAsGuest()).rejects.toMatchObject({ code: 'server_error' });
    expect(get(userStore.authState)).toBe('unauthenticated');
  });
});

describe('signUp and signIn', () => {
  it('enters the app right after registration without a second login', async () => {
    const { accountMock, userStore } = await loadModules();
    const user = makeUser();
    vi.mocked(accountMock.register).mockResolvedValue(user);

    await userStore.signUp({
      email: 'egor@example.com',
      username: 'egor',
      displayName: 'Egor',
      password: 'password',
    });

    expect(accountMock.login).not.toHaveBeenCalled();
    expect(get(userStore.authState)).toBe('authenticated');
    expect(get(userStore.currentUser)).toEqual(user);
  });

  it('enters the app after login and lands on the library route', async () => {
    const { accountMock, userStore, router } = await loadModules();
    router.navigate('settings');
    vi.mocked(accountMock.login).mockResolvedValue(makeUser());

    await userStore.signIn({ emailOrUsername: 'egor', password: 'password' });

    expect(get(userStore.authState)).toBe('authenticated');
    expect(get(router.route).name).toBe('library');
  });

  it('leaves the state unauthenticated and rethrows when credentials are wrong', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue({
      status: 'unauthenticated',
      user: emptyUser(),
      reason: '',
    } as never);
    await userStore.initAuth();

    vi.mocked(accountMock.login).mockRejectedValue(new accountMock.AccountError('invalid_credentials'));

    await expect(userStore.signIn({ emailOrUsername: 'egor', password: 'nope' })).rejects.toMatchObject({
      code: 'invalid_credentials',
    });
    expect(get(userStore.authState)).toBe('unauthenticated');
    expect(get(userStore.currentUser)).toBeNull();
  });
});

describe('signOut', () => {
  it('clears the user and returns to the login view', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.register).mockResolvedValue(makeUser());
    await userStore.signUp({
      email: 'egor@example.com',
      username: 'egor',
      displayName: 'Egor',
      password: 'password',
    });

    vi.mocked(accountMock.logout).mockResolvedValue(undefined);
    await userStore.signOut();

    expect(get(userStore.authState)).toBe('unauthenticated');
    expect(get(userStore.currentUser)).toBeNull();
    expect(get(userStore.authView)).toBe('login');
  });

  it('still signs the user out locally when the revoke call fails', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.register).mockResolvedValue(makeUser());
    await userStore.signUp({
      email: 'egor@example.com',
      username: 'egor',
      displayName: 'Egor',
      password: 'password',
    });

    vi.mocked(accountMock.logout).mockRejectedValue(new accountMock.AccountError('network_error'));
    await expect(userStore.signOut()).rejects.toMatchObject({ code: 'network_error' });

    expect(get(userStore.authState)).toBe('unauthenticated');
    expect(get(userStore.currentUser)).toBeNull();
  });
});

describe('saveProfile', () => {
  it('replaces currentUser with the server response on success', async () => {
    const { accountMock, userStore } = await loadModules();
    const updated = makeUser({ displayName: 'New Name' });
    vi.mocked(accountMock.updateProfile).mockResolvedValue(updated);

    await userStore.saveProfile({ displayName: 'New Name' });

    expect(get(userStore.currentUser)).toEqual(updated);
  });

  it('leaves currentUser untouched and rethrows AccountError on failure', async () => {
    const { accountMock, userStore } = await loadModules();
    const original = makeUser();
    userStore.currentUser.set(original);
    const err = new accountMock.AccountError('username_taken', 'username');
    vi.mocked(accountMock.updateProfile).mockRejectedValue(err);

    await expect(userStore.saveProfile({ username: 'taken' })).rejects.toMatchObject({
      code: 'username_taken',
    });

    expect(get(userStore.currentUser)).toEqual(original);
  });

  it('signs out when the session was rejected mid-session', async () => {
    const { accountMock, userStore } = await loadModules();
    userStore.currentUser.set(makeUser());
    userStore.authState.set('authenticated');
    vi.mocked(accountMock.updateProfile).mockRejectedValue(new accountMock.AccountError('unauthenticated'));

    await expect(userStore.saveProfile({ displayName: 'X' })).rejects.toMatchObject({
      code: 'unauthenticated',
    });

    expect(get(userStore.authState)).toBe('unauthenticated');
    expect(get(userStore.currentUser)).toBeNull();
  });

  it('issues no second request while one is already in flight', async () => {
    const { accountMock, userStore } = await loadModules();
    let resolveFirst!: (value: ReturnType<typeof makeUser>) => void;
    const first = new Promise<ReturnType<typeof makeUser>>((resolve) => {
      resolveFirst = resolve;
    });
    vi.mocked(accountMock.updateProfile).mockReturnValueOnce(first);

    const call1 = userStore.saveProfile({ displayName: 'A' });
    const call2 = userStore.saveProfile({ displayName: 'B' });

    resolveFirst(makeUser({ displayName: 'A' }));
    await Promise.all([call1, call2]);

    expect(accountMock.updateProfile).toHaveBeenCalledTimes(1);
  });
});

describe('chooseAvatar', () => {
  it('returns a data url and uploads nothing on its own', async () => {
    const { accountMock, userStore } = await loadModules();
    const original = makeUser({ avatarUrl: 'https://cdn/old.png' });
    userStore.currentUser.set(original);
    vi.mocked(accountMock.pickAvatar).mockResolvedValue({ data: 'QUJD', mime: 'image/png' });

    const src = await userStore.chooseAvatar();

    expect(src).toBe('data:image/png;base64,QUJD');
    expect(accountMock.uploadAvatar).not.toHaveBeenCalled();
    expect(get(userStore.currentUser)).toEqual(original);
  });

  it.each([
    ['cancelled dialog', { data: '', mime: '' }],
    ['payload without a type', { data: 'QUJD', mime: '' }],
    ['type without a payload', { data: '', mime: 'image/png' }],
  ])('returns nothing for %s', async (_name, image) => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.pickAvatar).mockResolvedValue(image);

    expect(await userStore.chooseAvatar()).toBe('');
    expect(accountMock.uploadAvatar).not.toHaveBeenCalled();
  });

  it('propagates a failure instead of returning an empty selection', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.pickAvatar).mockRejectedValue(new accountMock.AccountError('avatar_too_large'));

    await expect(userStore.chooseAvatar()).rejects.toMatchObject({ code: 'avatar_too_large' });
    expect(get(userStore.pickingAvatar)).toBe(false);
  });
});

describe('saveAvatar', () => {
  it('uploads the cropped payload and updates currentUser.avatarUrl', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.uploadAvatar).mockResolvedValue(makeUser({ avatarUrl: 'https://cdn/avatar.webp' }));

    await userStore.saveAvatar('QUJD');

    expect(accountMock.uploadAvatar).toHaveBeenCalledWith('QUJD');
    expect(get(userStore.currentUser)?.avatarUrl).toBe('https://cdn/avatar.webp');
  });

  it('refuses an empty payload', async () => {
    const { accountMock, userStore } = await loadModules();

    await expect(userStore.saveAvatar('')).rejects.toMatchObject({ code: 'invalid_avatar' });
    expect(accountMock.uploadAvatar).not.toHaveBeenCalled();
  });

  it('keeps the previous user and clears the flag when the upload fails', async () => {
    const { accountMock, userStore } = await loadModules();
    const original = makeUser({ avatarUrl: 'https://cdn/old.png' });
    userStore.currentUser.set(original);
    vi.mocked(accountMock.uploadAvatar).mockRejectedValue(new accountMock.AccountError('unsupported_avatar'));

    await expect(userStore.saveAvatar('QUJD')).rejects.toMatchObject({ code: 'unsupported_avatar' });
    expect(get(userStore.currentUser)).toEqual(original);
    expect(get(userStore.uploadingAvatar)).toBe(false);
  });
});

describe('deleteAvatar', () => {
  it('results in avatarUrl being empty', async () => {
    const { accountMock, userStore } = await loadModules();
    userStore.currentUser.set(makeUser({ avatarUrl: 'https://cdn/avatar.png' }));
    vi.mocked(accountMock.removeAvatar).mockResolvedValue(makeUser({ avatarUrl: '' }));

    await userStore.deleteAvatar();

    expect(get(userStore.currentUser)?.avatarUrl).toBe('');
  });
});

describe('offline mode and reconnect', () => {
  class FakeWindow {
    listeners = new Map<string, Array<(event: unknown) => void>>();

    addEventListener(type: string, handler: (event: unknown) => void) {
      const list = this.listeners.get(type) ?? [];
      list.push(handler);
      this.listeners.set(type, list);
    }

    removeEventListener(type: string, handler: (event: unknown) => void) {
      const next = (this.listeners.get(type) ?? []).filter((item) => item !== handler);
      if (next.length) this.listeners.set(type, next);
      else this.listeners.delete(type);
    }

    dispatchEvent(type: string) {
      for (const handler of this.listeners.get(type) ?? []) handler({ type });
      return true;
    }

    listenerCount(type: string) {
      return (this.listeners.get(type) ?? []).length;
    }
  }

  let win: FakeWindow;

  beforeEach(() => {
    win = new FakeWindow();
    vi.stubGlobal('window', win);
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  function offlineState() {
    return { status: 'offline', user: makeUser({ avatarUrl: 'data:image/webp;base64,AAAA' }), reason: 'network_error' };
  }

  it('enters offline mode with the cached profile when the server cannot be reached', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue(offlineState() as never);

    await userStore.initAuth();

    expect(get(userStore.authState)).toBe('offline');
    expect(get(userStore.isOffline)).toBe(true);
    expect(get(userStore.currentUser)?.username).toBe('egor');
    expect(get(userStore.currentUser)?.avatarUrl).toBe('data:image/webp;base64,AAAA');
    expect(get(userStore.authReason)).toBe('network_error');
    expect(win.listenerCount('online')).toBe(1);
  });

  it('retries bootstrap once the first backoff delay elapses while still offline', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue(offlineState() as never);

    await userStore.initAuth();
    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(14999);
    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1);
    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(2);
  });

  it('stops retrying once a retry restores the authenticated session', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession)
      .mockResolvedValueOnce(offlineState() as never)
      .mockResolvedValue({ status: 'authenticated', user: makeUser(), reason: '' } as never);

    await userStore.initAuth();
    await vi.advanceTimersByTimeAsync(15000);

    expect(get(userStore.authState)).toBe('authenticated');
    expect(get(userStore.authReason)).toBe('');
    expect(win.listenerCount('online')).toBe(0);

    const settled = vi.mocked(accountMock.bootstrapSession).mock.calls.length;
    await vi.advanceTimersByTimeAsync(120000);
    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(settled);
  });

  it('stops retrying and clears the cached user once the server rejects the credential', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession)
      .mockResolvedValueOnce(offlineState() as never)
      .mockResolvedValue({ status: 'unauthenticated', user: emptyUser(), reason: '' } as never);

    await userStore.initAuth();
    await vi.advanceTimersByTimeAsync(15000);

    expect(get(userStore.authState)).toBe('unauthenticated');
    expect(get(userStore.currentUser)).toBeNull();
    expect(win.listenerCount('online')).toBe(0);

    const settled = vi.mocked(accountMock.bootstrapSession).mock.calls.length;
    await vi.advanceTimersByTimeAsync(120000);
    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(settled);
  });

  it('retries immediately when the system reports the network is back', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.bootstrapSession).mockResolvedValue(offlineState() as never);

    await userStore.initAuth();
    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(1);

    win.dispatchEvent('online');
    await vi.advanceTimersByTimeAsync(0);

    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(2);
  });

  it('keeps a single pending retry when the network event races the backoff timer', async () => {
    const { accountMock, userStore } = await loadModules();
    let release!: (value: unknown) => void;
    vi.mocked(accountMock.bootstrapSession)
      .mockResolvedValueOnce(offlineState() as never)
      .mockReturnValueOnce(
        new Promise((r) => {
          release = r as (value: unknown) => void;
        }) as never,
      )
      .mockResolvedValue(offlineState() as never);

    await userStore.initAuth();
    await vi.advanceTimersByTimeAsync(15000);
    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(2);

    win.dispatchEvent('online');
    release(offlineState());
    await vi.advanceTimersByTimeAsync(0);

    const settled = vi.mocked(accountMock.bootstrapSession).mock.calls.length;
    await vi.advanceTimersByTimeAsync(60000);
    expect(accountMock.bootstrapSession).toHaveBeenCalledTimes(settled + 1);
  });
});
