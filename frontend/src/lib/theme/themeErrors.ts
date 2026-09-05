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
};

export function themeErrorText(err: unknown, fallback: string = msg('errLibrary.themeErrFallback')): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : fallback;
}
