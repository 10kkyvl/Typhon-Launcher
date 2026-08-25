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
});
