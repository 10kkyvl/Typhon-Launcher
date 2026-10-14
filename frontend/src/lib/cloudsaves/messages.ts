import { errorCode, hasMessage, msg, type MessageKey } from '../i18n';
import type { CloudState } from '../services/cloudsaves';

const STATE_KEYS: Record<CloudState, MessageKey> = {
  synced: 'cloudsaves.stateSynced',
  pending: 'cloudsaves.statePending',
  conflict: 'cloudsaves.stateConflict',
  failed: 'cloudsaves.stateFailed',
  skipped: 'cloudsaves.stateSkipped',
};

export function cloudCodeText(code: string, fallback: string): string {
  return code.startsWith('cloud.') && hasMessage(code) ? msg(code) : fallback;
}

export function cloudErrorText(err: unknown, fallback: string = msg('cloudsaves.fallback')): string {
  return cloudCodeText(errorCode(err), fallback);
}

export function cloudStateLabel(state: CloudState): string {
  const key = STATE_KEYS[state];
  return key ? msg(key) : state;
}

export function cloudStateKind(state: CloudState): 'success' | 'warning' | 'danger' | 'neutral' {
  if (state === 'synced') return 'success';
  if (state === 'conflict' || state === 'skipped') return 'warning';
  if (state === 'failed') return 'danger';
  return 'neutral';
}
