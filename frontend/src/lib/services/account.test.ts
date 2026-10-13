import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  Bootstrap: vi.fn(),
  Register: vi.fn(),
  Login: vi.fn(),
  Logout: vi.fn(),
  GetCurrentUser: vi.fn(),
  UpdateProfile: vi.fn(),
  PickAvatar: vi.fn(),
  UploadAvatar: vi.fn(),
  RemoveAvatar: vi.fn(),
  OpenForgotPassword: vi.fn(),
  OpenSupportSite: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/account', () => ({ Service: bindings }));
vi.mock('./backend', () => ({ inWails: true }));

const user = {
  id: '1',
  username: 'alex_123',
  displayName: 'Алексей',
  email: 'a@b.c',
  avatarUrl: '',
  createdAt: '2026-01-01T00:00:00Z',
};

describe('account service errors', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it.each([
    ['username_taken', 'username'],
    ['invalid_username', 'username'],
    ['invalid_display_name', 'displayName'],
    ['email_immutable', 'email'],
    ['avatar_too_large', 'avatar'],
    ['unsupported_avatar', 'avatar'],
    ['invalid_avatar', 'avatar'],
    ['unauthenticated', ''],
  ])('maps %s to field %s', async (code, field) => {
    const { AccountError, updateProfile } = await import('./account');
    bindings.UpdateProfile.mockRejectedValueOnce(new Error(code));
    const err = await updateProfile({ username: 'x' }).catch((e) => e);
    expect(err).toBeInstanceOf(AccountError);
    expect(err.code).toBe(code);
    expect(err.field).toBe(field);
  });

  it.each([
    ['email_taken', 'email'],
    ['invalid_email', 'email'],
    ['invalid_password', 'password'],
    ['rate_limited', ''],
    ['auth_busy', ''],
    ['invalid_credentials', ''],
  ])('maps auth error %s to field %s', async (code, field) => {
    const { AccountError, login } = await import('./account');
    bindings.Login.mockRejectedValueOnce(new Error(code));
    const err = await login({ emailOrUsername: 'x', password: 'y' }).catch((e) => e);
    expect(err).toBeInstanceOf(AccountError);
    expect(err.code).toBe(code);
    expect(err.field).toBe(field);
  });

  it('falls back to server_error for unknown messages', async () => {
    const { updateProfile } = await import('./account');
    bindings.UpdateProfile.mockRejectedValueOnce(new Error('boom'));
    await expect(updateProfile({})).rejects.toMatchObject({ code: 'server_error', field: '' });
  });

  it('returns the server response unchanged', async () => {
    const { updateProfile } = await import('./account');
    bindings.UpdateProfile.mockResolvedValueOnce(user);
    await expect(updateProfile({ username: 'Alex_123' })).resolves.toEqual(user);
    expect(bindings.UpdateProfile).toHaveBeenCalledWith({ username: 'Alex_123' });
  });
});

describe('updateProfile profile payload', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  async function sentProfile(profile: Record<string, unknown>) {
    const { updateProfile, DEFAULT_PROFILE } = await import('./account');
    bindings.UpdateProfile.mockResolvedValueOnce(user);
    await updateProfile({ profile: { ...DEFAULT_PROFILE, ...profile } as never });
    return bindings.UpdateProfile.mock.calls[0][0].profile as Record<string, unknown>;
  }

  it('does not invent an appearance when the caller sent none', async () => {
    const sent = await sentProfile({ visibility: 'private' });

    expect(Object.keys(sent)).not.toContain('appearance');
    expect(sent.visibility).toBe('private');
  });

  it('treats a null appearance as none', async () => {
    const sent = await sentProfile({ appearance: null });

    expect(Object.keys(sent)).not.toContain('appearance');
  });

  it('sends a normalised appearance when the caller sent one', async () => {
    const sent = await sentProfile({ appearance: { theme: 'forest', coverDim: 500 } });

    expect(sent.appearance).toMatchObject({ theme: 'forest', coverDim: 100, accent: '#67d8ef' });
  });

  it('keeps an empty showcase as a value and an absent one as absent', async () => {
    const { updateProfile } = await import('./account');
    bindings.UpdateProfile.mockResolvedValue(user);
    await updateProfile({ profile: { visibility: 'friends', showOnline: true, showPlaying: true, showPlaytime: true, showLibrary: true, showActivity: true, showStats: true, showcase: [] } });
    await updateProfile({ profile: { visibility: 'friends', showOnline: true, showPlaying: true, showPlaytime: true, showLibrary: true, showActivity: true, showStats: true } as never });

    expect(bindings.UpdateProfile.mock.calls[0][0].profile.showcase).toEqual([]);
    expect(Object.keys(bindings.UpdateProfile.mock.calls[1][0].profile)).not.toContain('showcase');
  });
});

describe('auth calls', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('sends the registration payload untouched', async () => {
    const { register } = await import('./account');
    const input = {
      email: '  User@Example.COM ',
      username: 'PlayerOne',
      displayName: 'Алексей 🎮',
      password: '  secret pass  ',
    };
    bindings.Register.mockResolvedValueOnce(user);

    await expect(register(input)).resolves.toEqual(user);
    expect(bindings.Register).toHaveBeenCalledWith(input);
  });

  it('sends the login payload untouched', async () => {
    const { login } = await import('./account');
    const input = { emailOrUsername: 'PLAYERONE', password: '  secret pass  ' };
    bindings.Login.mockResolvedValueOnce(user);

    await expect(login(input)).resolves.toEqual(user);
    expect(bindings.Login).toHaveBeenCalledWith(input);
  });

  it('logs out through the backend', async () => {
    const { logout } = await import('./account');
    bindings.Logout.mockResolvedValueOnce(undefined);

    await expect(logout()).resolves.toBeUndefined();
    expect(bindings.Logout).toHaveBeenCalledTimes(1);
  });
});

describe.each([
  ['openForgotPassword', 'OpenForgotPassword'],
  ['openSupportSite', 'OpenSupportSite'],
] as const)('%s', (name, binding) => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('asks the backend to open the page', async () => {
    const service = await import('./account');
    bindings[binding].mockResolvedValueOnce(undefined);

    await expect(service[name]()).resolves.toBeUndefined();
    expect(bindings[binding]).toHaveBeenCalledTimes(1);
  });

  it('surfaces a browser failure as an AccountError instead of swallowing it', async () => {
    const service = await import('./account');
    bindings[binding].mockRejectedValueOnce(new Error('open page: no default browser'));

    const err = await service[name]().catch((e) => e);
    expect(err).toBeInstanceOf(service.AccountError);
    expect(err.code).toBe('server_error');
  });
});

describe('bootstrapSession', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it.each([
    ['authenticated', ''],
    ['unauthenticated', ''],
    ['unavailable', 'network_error'],
  ])('passes through the %s state', async (status, reason) => {
    const { bootstrapSession } = await import('./account');
    bindings.Bootstrap.mockResolvedValueOnce({ status, user, reason });

    await expect(bootstrapSession()).resolves.toEqual({ status, user, reason });
  });

  it('tolerates a response without user or reason', async () => {
    const { bootstrapSession } = await import('./account');
    bindings.Bootstrap.mockResolvedValueOnce({ status: 'unauthenticated' });

    const state = await bootstrapSession();
    expect(state.status).toBe('unauthenticated');
    expect(state.reason).toBe('');
    expect(state.user.id).toBe('');
  });

  it('maps a thrown backend error to an AccountError', async () => {
    const { AccountError, bootstrapSession } = await import('./account');
    bindings.Bootstrap.mockRejectedValueOnce(new Error('network_error'));

    const err = await bootstrapSession().catch((e) => e);
    expect(err).toBeInstanceOf(AccountError);
    expect(err.code).toBe('network_error');
  });
});

describe('accountMessage', () => {
  it('has its own text for rate_limited instead of the generic fallback', async () => {
    const { accountMessage } = await import('./accountMessages');
    const text = accountMessage('rate_limited', 'FALLBACK');
    expect(text).not.toBe('FALLBACK');
    expect(text).toContain('попыток');
  });

  it('names a suspended account and an undeliverable address instead of blaming the server', async () => {
    const { accountMessage } = await import('./accountMessages');
    const { toAccountError } = await import('./account');
    for (const code of ['account_suspended', 'email_undeliverable', 'credential_store_error']) {
      expect(toAccountError(new Error(code)).code).toBe(code);
      const text = accountMessage(code, 'FALLBACK');
      expect(text).not.toBe('FALLBACK');
      expect(text).not.toBe(accountMessage('server_error'));
    }
    expect(toAccountError(new Error('email_undeliverable')).field).toBe('email');
  });

  it('tells a busy sign-in apart from a wrong password', async () => {
    const { accountMessage } = await import('./accountMessages');
    const text = accountMessage('auth_busy', 'FALLBACK');
    expect(text).not.toBe('FALLBACK');
    expect(text).not.toBe(accountMessage('invalid_credentials'));
    expect(text).toContain('повторите');
  });
});

describe('uploadAvatar', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('sends the framed square along with the image', async () => {
    const { uploadAvatar } = await import('./account');
    bindings.UploadAvatar.mockResolvedValueOnce(user);

    await uploadAvatar('QUJD', { x: 12, y: 34, size: 300 });

    expect(bindings.UploadAvatar).toHaveBeenCalledWith('QUJD', { x: 12, y: 34, size: 300 });
  });

  it('leaves the square to the server when nothing was framed', async () => {
    const { uploadAvatar } = await import('./account');
    bindings.UploadAvatar.mockResolvedValueOnce(user);

    await uploadAvatar('QUJD');

    expect(bindings.UploadAvatar).toHaveBeenCalledWith('QUJD', { x: 0, y: 0, size: 0 });
  });
});
