import { get } from 'svelte/store';
import { locale } from '../i18n/locale';
import { Service as AccountService } from '../../../bindings/typhon/internal/account';
import { appearanceOf } from '../profile/appearance';
import { inWails } from './backend';
import type { CropRect } from '../utils/crop';

export const SHOWCASE_KINDS = ['favorites', 'recently_completed', 'most_played'] as const;
export type ShowcaseKind = (typeof SHOWCASE_KINDS)[number];

export const VISIBILITIES = ['public', 'friends', 'private'] as const;
export type Visibility = (typeof VISIBILITIES)[number];

export const AVATAR_FRAMES = ['none', 'ring', 'glow', 'neon', 'orbit', 'pixel'] as const;
export type AvatarFrame = (typeof AVATAR_FRAMES)[number];

export const NAME_STYLES = ['plain', 'gradient', 'glow'] as const;
export type NameStyle = (typeof NAME_STYLES)[number];

export const AUTO_SOURCES = ['playing', 'pinned', 'most_played'] as const;
export type AutoSource = (typeof AUTO_SOURCES)[number];

export interface ProfileAppearance {
  theme: string;
  accent: string;
  coverUrl: string;
  coverDim: number;
  coverPosition: number;
  customFrom: string;
  customTo: string;
  customAngle: number;
  autoSource: AutoSource;
  avatarFrame: AvatarFrame;
  nameStyle: NameStyle;
  parallax: boolean;
}

export const BLOCK_TYPES = [
  'pinned', 'collection', 'genres', 'fingerprint', 'text', 'recent', 'activity', 'stats', 'playing', 'about',
] as const;
export type BlockType = (typeof BLOCK_TYPES)[number];

export const BLOCK_WIDTHS = ['full', 'half', 'third'] as const;
export type BlockWidth = (typeof BLOCK_WIDTHS)[number];

export const COLLECTION_SOURCES = ['manual', 'favorites', 'recently_completed', 'most_played'] as const;
export type CollectionSource = (typeof COLLECTION_SOURCES)[number];

export const MAX_BLOCKS = 16;
export const MAX_TEXT_BLOCKS = 4;
export const MAX_COLLECTION_GAMES = 12;
export const MAX_CAPTION = 140;
export const MAX_BLOCK_TITLE = 40;
export const MAX_TEXT_BODY = 1000;
export const MAX_STATUS_TEXT = 60;

export interface PinnedConfig { igdbId: number; caption: string }
export interface CollectionConfig { source: CollectionSource; title?: string; igdbIds?: number[] }
export interface TextConfig { title: string; body: string }

export interface ProfileBlock {
  id: string;
  type: string;
  width: BlockWidth;
  config: Record<string, unknown>;
}

export interface ProfileLayout {
  version: 1;
  blocks: ProfileBlock[];
}

export interface ProfileSettings {
  appearance?: ProfileAppearance;
  visibility: Visibility;
  showOnline: boolean;
  showPlaying: boolean;
  showPlaytime: boolean;
  showLibrary: boolean;
  showActivity: boolean;
  showStats: boolean;
  showcase: ShowcaseKind[];
  statusEmoji?: string;
  statusText?: string;
  layout?: ProfileLayout | null;
}

export const DEFAULT_PROFILE: ProfileSettings = {
  visibility: 'friends',
  showOnline: true,
  showPlaying: true,
  showPlaytime: true,
  showLibrary: true,
  showActivity: true,
  showStats: true,
  showcase: ['favorites'],
};

export interface CurrentUser {
  id: string;
  username: string;
  displayName: string;
  email: string;
  avatarUrl: string;
  bio: string;
  profile: ProfileSettings;
  createdAt: string;
}

export interface AvatarImage {
  data: string;
  mime: string;
}

export interface ProfilePatch {
  username?: string;
  displayName?: string;
  bio?: string;
  profile?: ProfileSettings;
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

export type AuthStatus = 'authenticated' | 'unauthenticated' | 'unavailable' | 'guest' | 'offline';

export interface BootstrapState {
  status: AuthStatus;
  user: CurrentUser;
  reason: string;
}

const KNOWN_CODES = new Set([
  'unauthenticated',
  'sync_disabled',
  'invalid_credentials',
  'username_taken',
  'email_taken',
  'invalid_email',
  'invalid_password',
  'invalid_username',
  'invalid_display_name',
  'email_immutable',
  'launcher_outdated',
  'no_changes',
  'cover_too_large',
  'unsupported_cover',
  'invalid_cover',
  'avatar_too_large',
  'unsupported_avatar',
  'invalid_avatar',
  'invalid_profile',
  'invalid_bio',
  'rate_limited',
  'bad_request',
  'request_blocked',
  'user_not_found',
  'unknown_game',
  'already_friends',
  'friend_limit',
  'request_limit',
  'block_limit',
  'friend_self',
  'no_request',
  'not_friends',
  'internal',
  'network_error',
  'server_error',
  'account_muted',
  'review_not_found',
  'review_too_short',
  'review_too_long',
  'review_low_effort',
  'review_links',
  'review_duplicate',
  'review_not_played',
  'review_account_too_new',
  'review_own',
  'review_post_cooldown',
  'review_daily_limit',
  'review_repost_cooldown',
  'review_edit_cooldown',
  'review_report_limit',
  'review_bad_reason',
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
  invalid_profile: 'profile',
  invalid_bio: 'bio',
  friend_self: 'query',
  review_too_short: 'body',
  review_too_long: 'body',
  review_low_effort: 'body',
  review_links: 'body',
  review_duplicate: 'body',
  review_bad_reason: 'reason',
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

export function toAccountError(err: unknown): AccountError {
  if (err instanceof AccountError) return err;
  const raw = err instanceof Error ? err.message : String(err);
  if (KNOWN_CODES.has(raw)) return new AccountError(raw);
  console.error('account call failed outside the error contract', raw, err);
  return new AccountError('server_error');
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
  return {
    id: '',
    username: '',
    displayName: '',
    email: '',
    avatarUrl: '',
    bio: '',
    profile: DEFAULT_PROFILE,
    createdAt: '',
  };
}

export async function continueAsGuest(): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await AccountService.ContinueAsGuest();
  } catch (err) {
    throw toAccountError(err);
  }
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

export async function deleteAccount(password: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await AccountService.DeleteAccount(password);
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
    return (await AccountService.UpdateProfile({ ...patch, profile: patch.profile ? { ...patch.profile, appearance: appearanceOf(patch.profile.appearance) } : undefined })) as CurrentUser;
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function pickAvatar(): Promise<AvatarImage> {
  if (!inWails) throw unauthenticated();
  try {
    const image = (await AccountService.PickAvatar(get(locale))) as unknown as AvatarImage | null;
    if (!image) throw new AccountError('server_error');
    return { data: image.data ?? '', mime: image.mime ?? '' };
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function uploadAvatar(encoded: string, crop: CropRect = { x: 0, y: 0, size: 0 }): Promise<CurrentUser> {
  if (!inWails) throw unauthenticated();
  try {
    return (await AccountService.UploadAvatar(encoded, crop)) as CurrentUser;
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
