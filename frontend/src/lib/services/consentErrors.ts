import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';

// Codes only, never a russian-text substring match: see internal/uierr and
// ErrCodeConsentSaveFailed in internal/settings/settings.go. The consent
// screen closes only on a successful answer, so whatever this returns is the
// last thing the user reads before trying again.
const REASONS: Record<string, MessageKey> = {
  'settings.consent_save_failed': 'modals.telemetryConsentSaveFailed',
};

export function consentErrorText(err: unknown): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : msg('modals.telemetryConsentSaveFallback');
}
