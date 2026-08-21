import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  GetCurrentUser: vi.fn(),
  UpdateProfile: vi.fn(),
  SelectAvatarFile: vi.fn(),
  UploadAvatar: vi.fn(),
  RemoveAvatar: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/account', () => ({ Service: bindings }));
vi.mock('./backend', () => ({ inWails: true }));

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

  it('falls back to server_error for unknown messages', async () => {
    const { updateProfile } = await import('./account');
    bindings.UpdateProfile.mockRejectedValueOnce(new Error('boom'));
    await expect(updateProfile({})).rejects.toMatchObject({ code: 'server_error', field: '' });
  });

  it('returns the server response unchanged', async () => {
    const { updateProfile } = await import('./account');
    const user = {
      id: '1',
      username: 'alex_123',
      displayName: 'Алексей',
      email: 'a@b.c',
      avatarUrl: '',
      createdAt: '2026-01-01T00:00:00Z',
    };
    bindings.UpdateProfile.mockResolvedValueOnce(user);
    await expect(updateProfile({ username: 'Alex_123' })).resolves.toEqual(user);
    expect(bindings.UpdateProfile).toHaveBeenCalledWith({ username: 'Alex_123' });
  });
});
