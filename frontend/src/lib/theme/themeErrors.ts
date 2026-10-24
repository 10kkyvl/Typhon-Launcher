import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';

export const REASONS: Record<string, MessageKey> = {
  'theme.file_too_large': 'theme.file_too_large',
  'theme.unsupported_version': 'theme.unsupported_version',
  'theme.invalid_id': 'theme.invalid_id',
  'theme.invalid_name': 'theme.invalid_name',
  'theme.invalid_base': 'theme.invalid_base',
  'theme.unknown_token': 'theme.unknown_token',
  'theme.invalid_token_value': 'theme.invalid_token_value',
  'theme.css_too_large': 'theme.css_too_large',
  'theme.css_forbidden': 'theme.css_forbidden',
  'theme.css_unbalanced_braces': 'theme.css_unbalanced_braces',
  'theme.built_in': 'theme.built_in',
  'theme.not_found': 'theme.not_found',
  'theme.invalid_path': 'theme.invalid_path',
  'theme.dialog_unavailable': 'theme.dialog_unavailable',
  'theme.token_kind': 'theme.token_kind',
  'theme.skill_invalid_path': 'theme.skill_invalid_path',
  'theme.skill_write_failed': 'theme.skill_write_failed',
};

const TOKEN = /--[a-z0-9-]+/g;

function tokenNames(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  const subject = raw.split('typhon:')[0];
  return [...new Set(subject.match(TOKEN) ?? [])].join(', ');
}

export function themeErrorText(err: unknown, fallback: string = msg('errLibrary.themeErrFallback')): string {
  const code = errorCode(err);
  const key = REASONS[code];
  if (!key) return fallback;
  if (code === 'theme.token_kind') {
    const name = tokenNames(err);
    return name ? `${msg(key)}: ${name}` : msg(key);
  }
  return msg(key);
}
