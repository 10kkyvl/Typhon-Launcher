import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => vi.fn()) },
  Call: { ByID: vi.fn() },
  CancellablePromise: class {},
}));
vi.mock('../services/backend', () => ({ inWails: true }));
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
vi.mock('../services/online', () => ({
  DEFAULT_PRESENCE: 'online',
  PRESENCE_STATUSES: ['online', 'away', 'busy', 'invisible'],
  toPresenceStatus: (value: string) => value,
  status: vi.fn(async () => 'online'),
  setStatus: vi.fn(async () => {}),
  kick: vi.fn(async () => {}),
}));

async function load() {
  vi.resetModules();
  const presence = await import('./presence');
  const { authState } = await import('./user');
  await presence.initPresence();
  authState.set('authenticated');
  return { ...presence, authState };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('updatePresenceStatus', () => {
  it('показывает новый статус, не дожидаясь бэкенда', async () => {
    const { presenceStatus, updatePresenceStatus } = await load();
    const { setStatus } = await import('../services/online');
    let finish: () => void = () => {};
    vi.mocked(setStatus).mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );

    const pending = updatePresenceStatus('busy');
    expect(get(presenceStatus)).toBe('busy');

    finish();
    await pending;

    expect(get(presenceStatus)).toBe('busy');
    expect(setStatus).toHaveBeenCalledWith('busy');
  });

  it('возвращает прежний статус, когда бэкенд отказал', async () => {
    const { presenceStatus, updatePresenceStatus } = await load();
    const { setStatus } = await import('../services/online');
    vi.mocked(setStatus).mockRejectedValueOnce(new Error('нет связи'));

    await expect(updatePresenceStatus('away')).rejects.toThrow('нет связи');

    expect(get(presenceStatus)).toBe('online');
  });

  it('не ходит на бэкенд за тем же статусом', async () => {
    const { updatePresenceStatus } = await load();
    const { setStatus } = await import('../services/online');

    await updatePresenceStatus('online');

    expect(setStatus).not.toHaveBeenCalled();
  });
});

describe('вход в аккаунт', () => {
  it('просит бэкенд отправить присутствие заново', async () => {
    const { authState } = await load();
    const { kick } = await import('../services/online');
    authState.set('unauthenticated');
    vi.mocked(kick).mockClear();

    authState.set('authenticated');

    expect(kick).toHaveBeenCalledTimes(1);
  });
});
