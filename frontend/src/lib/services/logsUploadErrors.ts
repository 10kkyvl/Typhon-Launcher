import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';

// Codes only, never a russian-text substring match: see internal/uierr and
// the ErrCodeLogUpload* constants in internal/diagnostics/logsupload.go.
export const REASONS: Record<string, MessageKey> = {
  'diagnostics.log_upload_too_large': 'errLogs.uploadTooLarge',
  'diagnostics.log_upload_rate_limited': 'errLogs.uploadRateLimited',
  'diagnostics.log_upload_timeout': 'errLogs.uploadTimeout',
  'diagnostics.log_upload_cancelled': 'errLogs.uploadCancelled',
  'diagnostics.log_upload_network': 'errLogs.uploadNetwork',
  'diagnostics.log_upload_unavailable': 'errLogs.uploadUnavailable',
  'diagnostics.log_upload_rejected': 'errLogs.uploadRejected',
  'diagnostics.log_upload_failed': 'errLogs.uploadFailed',
  'diagnostics.log_upload_busy': 'errLogs.uploadBusy',
};

export function logsUploadErrorText(err: unknown, fallback: string = msg('errLogs.uploadFallback')): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : fallback;
}
