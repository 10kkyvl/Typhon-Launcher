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

async function loadModules() {
  vi.resetModules();
  const accountMock = await import('../services/account');
  const userStore = await import('./user');
  return { accountMock, userStore };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('initCurrentUser', () => {
  it('calls fetchCurrentUser exactly once even when invoked twice', async () => {
    const { accountMock, userStore } = await loadModules();
    const user = makeUser();
    vi.mocked(accountMock.fetchCurrentUser).mockResolvedValue(user);

    await Promise.all([userStore.initCurrentUser(), userStore.initCurrentUser()]);

    expect(accountMock.fetchCurrentUser).toHaveBeenCalledTimes(1);
    expect(get(userStore.currentUser)).toEqual(user);
  });

  it('clears currentUser on an unauthenticated response', async () => {
    const { accountMock, userStore } = await loadModules();
    const err = new accountMock.AccountError('unauthenticated');
    vi.mocked(accountMock.fetchCurrentUser).mockRejectedValue(err);

    userStore.currentUser.set(makeUser());
    await userStore.initCurrentUser();

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
