import { msg } from '../i18n';
import { AccountError } from './account';

function messages(): Record<string, string> {
  return {
    invalid_credentials: msg('state.accountErrorInvalidCredentials'),
    username_taken: msg('state.accountErrorUsernameTaken'),
    email_taken: msg('state.accountErrorEmailTaken'),
    invalid_username: msg('state.accountErrorInvalidUsername'),
    invalid_display_name: msg('state.accountErrorInvalidDisplayName'),
    invalid_email: msg('state.accountErrorInvalidEmail'),
    invalid_password: msg('state.accountErrorInvalidPassword'),
    email_immutable: msg('state.accountErrorEmailImmutable'),
    launcher_outdated: msg('state.accountErrorLauncherOutdated'),
    no_changes: msg('state.accountErrorNoChanges'),
    avatar_too_large: msg('state.accountErrorAvatarTooLarge'),
    unsupported_avatar: msg('state.accountErrorUnsupportedAvatar'),
    invalid_avatar: msg('state.accountErrorInvalidAvatar'),
    invalid_profile: msg('state.accountErrorInvalidProfile'),
    invalid_bio: msg('state.accountErrorInvalidBio'),
    unauthenticated: msg('state.accountErrorUnauthenticated'),
    sync_disabled: msg('state.accountErrorSyncDisabled'),
    network_error: msg('state.accountErrorNetwork'),
    rate_limited: msg('state.accountErrorRateLimited'),
    bad_request: msg('state.accountErrorBadRequest'),
    request_blocked: msg('state.accountErrorRequestBlocked'),
    user_not_found: msg('state.accountErrorUserNotFound'),
    already_friends: msg('state.accountErrorAlreadyFriends'),
    friend_limit: msg('state.accountErrorLimitReached'),
    request_limit: msg('state.accountErrorLimitReached'),
    block_limit: msg('state.accountErrorLimitReached'),
    friend_self: msg('state.accountErrorFriendSelf'),
    no_request: msg('state.accountErrorNoRequest'),
    not_friends: msg('state.accountErrorNotFriends'),
    activity_not_found: msg('state.accountErrorActivityNotFound'),
    reaction_invalid: msg('state.accountErrorReactionInvalid'),
    internal: msg('state.accountErrorInternal'),
    server_error: msg('state.accountErrorServerError'),
  };
}

export function accountMessage(code: string, fallback = msg('state.accountErrorGeneric')): string {
  return messages()[code] ?? fallback;
}

export function accountErrorText(err: unknown, fallback?: string): string {
  const code = err instanceof AccountError ? err.code : '';
  return accountMessage(code, fallback);
}

export function accountErrorField(err: unknown): string {
  return err instanceof AccountError ? err.field : '';
}
