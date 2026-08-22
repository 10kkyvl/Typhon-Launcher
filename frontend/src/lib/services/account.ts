import { Service as AccountService } from '../../../bindings/typhon/internal/account';
import { inWails } from './backend';

export interface CurrentUser {
  id: string;
  username: string;
  displayName: string;
  email: string;
  avatarUrl: string;
  createdAt: string;
}

export interface ProfilePatch {
  username?: string;
  displayName?: string;
}

export interface RegisterInput {
  email: string;
  username: string;
  displayName: string;
  password: string;
}

export interface LoginInput {
  emailOrUsername: string;
  password: string;
}

export type AuthStatus = 'authenticated' | 'unauthenticated' | 'unavailable';

export interface BootstrapState {
  status: AuthStatus;
  user: CurrentUser;
  reason: string;
}

const KNOWN_CODES = new Set([
  'unauthenticated',
  'invalid_credentials',
  'username_taken',
  'email_taken',
  'invalid_email',
  'invalid_password',
  'invalid_username',
  'invalid_display_name',
  'email_immutable',
  'no_changes',
  'avatar_too_large',
  'unsupported_avatar',
  'invalid_avatar',
  'bad_request',
  'internal',
  'network_error',
  'server_error',
]);

const CODE_FIELDS: Record<string, string> = {
  username_taken: 'username',
  invalid_username: 'username',
  invalid_display_name: 'displayName',
  email_taken: 'email',
  invalid_email: 'email',
  email_immutable: 'email',
  invalid_password: 'password',
  avatar_too_large: 'avatar',
  unsupported_avatar: 'avatar',
  invalid_avatar: 'avatar',
};

export class AccountError extends Error {
  code: string;
  field: string;

  constructor(code: string, field = CODE_FIELDS[code] ?? '') {
    super(code);
    this.name = 'AccountError';
    this.code = code;
    this.field = field;
  }
}

function toAccountError(err: unknown): AccountError {
  if (err instanceof AccountError) return err;
  const raw = err instanceof Error ? err.message : String(err);
  const code = KNOWN_CODES.has(raw) ? raw : 'server_error';
  return new AccountError(code);
}

const unauthenticated = () => new AccountError('unauthenticated');

export async function bootstrapSession(): Promise<BootstrapState> {
  if (!inWails) {
    return { status: 'unauthenticated', user: emptyUser(), reason: '' };
  }
  try {
    const state = (await AccountService.Bootstrap()) as unknown as BootstrapState | null;
    if (!state) throw new AccountError('server_error');
    return { status: state.status, user: state.user ?? emptyUser(), reason: state.reason ?? '' };
  } catch (err) {
    throw toAccountError(err);
  }
}

function emptyUser(): CurrentUser {
  return { id: '', username: '', displayName: '', email: '', avatarUrl: '', createdAt: '' };
}

export async function register(input: RegisterInput): Promise<CurrentUser> {
  if (!inWails) throw unauthenticated();
  try {
    return (await AccountService.Register(input)) as unknown as CurrentUser;
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function login(input: LoginInput): Promise<CurrentUser> {
  if (!inWails) throw unauthenticated();
  try {
    return (await AccountService.Login(input)) as unknown as CurrentUser;
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function logout(): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await AccountService.Logout();
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function fetchCurrentUser(): Promise<CurrentUser> {
  if (!inWails) throw unauthenticated();
  try {
    return (await AccountService.GetCurrentUser()) as CurrentUser;
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function updateProfile(patch: ProfilePatch): Promise<CurrentUser> {
  if (!inWails) throw unauthenticated();
  try {
    return (await AccountService.UpdateProfile(patch)) as CurrentUser;
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function selectAvatarFile(): Promise<string> {
  if (!inWails) throw unauthenticated();
  try {
    return await AccountService.SelectAvatarFile();
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function uploadAvatar(path: string): Promise<CurrentUser> {
  if (!inWails) throw unauthenticated();
  try {
    return (await AccountService.UploadAvatar(path)) as CurrentUser;
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function removeAvatar(): Promise<CurrentUser> {
  if (!inWails) throw unauthenticated();
  try {
    return (await AccountService.RemoveAvatar()) as CurrentUser;
  } catch (err) {
    throw toAccountError(err);
  }
}
