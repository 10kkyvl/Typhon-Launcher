import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

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
    register: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(),
    fetchCurrentUser: vi.fn(),
    updateProfile: vi.fn(),
    selectAvatarFile: vi.fn(),
    uploadAvatar: vi.fn(),
    removeAvatar: vi.fn(),
  };
});

function makeUser(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: 'u1',
    username: 'egor',
    displayName: 'Egor',
    email: 'egor@example.com',
    avatarUrl: '',
    createdAt: '2024-01-01T00:00:00Z',
    ...overrides,
  };
}

function emptyUser() {
  return { id: '', username: '', displayName: '', email: '', avatarUrl: '', createdAt: '' };
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

describe('changeAvatar', () => {
  it('updates currentUser.avatarUrl', async () => {
    const { accountMock, userStore } = await loadModules();
    vi.mocked(accountMock.selectAvatarFile).mockResolvedValue('C:/avatar.png');
    vi.mocked(accountMock.uploadAvatar).mockResolvedValue(makeUser({ avatarUrl: 'https://cdn/avatar.png' }));

    await userStore.changeAvatar();

    expect(get(userStore.currentUser)?.avatarUrl).toBe('https://cdn/avatar.png');
  });

  it('keeps the current user when the file dialog is cancelled', async () => {
    const { accountMock, userStore } = await loadModules();
    const original = makeUser({ avatarUrl: 'https://cdn/old.png' });
    userStore.currentUser.set(original);
    vi.mocked(accountMock.selectAvatarFile).mockResolvedValue('');

    await userStore.changeAvatar();

    expect(accountMock.uploadAvatar).not.toHaveBeenCalled();
    expect(get(userStore.currentUser)).toEqual(original);
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
