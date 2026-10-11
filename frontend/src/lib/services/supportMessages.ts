import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';
import { AccountError } from './account';
import { accountMessage } from './accountMessages';
import { logsUploadErrorText } from './logsUploadErrors';

export const REASONS: Record<string, MessageKey> = {
  ticket_not_found: 'errSupport.notFound',
  ticket_invalid_topic: 'errSupport.invalidTopic',
  ticket_invalid_subject: 'errSupport.invalidSubject',
  ticket_invalid_body: 'errSupport.invalidBody',
  ticket_invalid_game: 'errSupport.invalidGame',
  ticket_invalid_client: 'errSupport.invalidClient',
  ticket_limit: 'errSupport.limit',
  ticket_bundle_limit: 'errSupport.bundleLimit',
  ticket_bundle_unavailable: 'errSupport.bundleUnavailable',
};

const LOGS_UNAVAILABLE = 'support.logs_unavailable';

export function supportErrorText(err: unknown, fallback: string = msg('errSupport.generic')): string {
  const coded = errorCode(err);
  if (coded === LOGS_UNAVAILABLE) return msg('errSupport.logsUnavailable');
  if (coded.startsWith('diagnostics.')) return logsUploadErrorText(err, msg('errSupport.logsFailed'));

  const code = err instanceof AccountError ? err.code : err instanceof Error ? err.message : String(err ?? '');
  const key = REASONS[code];
  return key ? msg(key) : accountMessage(code, fallback);
}
