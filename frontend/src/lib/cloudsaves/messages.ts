import { errorCode, hasMessage, msg, type MessageKey } from '../i18n';
import type { CloudState } from '../services/cloudsaves';

const STATE_KEYS: Record<CloudState, MessageKey> = {
  synced: 'cloudsaves.stateSynced',
  pending: 'cloudsaves.statePending',
  conflict: 'cloudsaves.stateConflict',
  failed: 'cloudsaves.stateFailed',
  skipped: 'cloudsaves.stateSkipped',
};

// The codes that say the cloud cannot settle anything now: the server is out of reach, or the session it
// has to confirm is not confirmed yet, which is what every request answers while a launcher started
// without a connection has not reached the server. The player reads one reason for both: the
// `cloud.network` text. The status says the same through `error` and `pending` (see CloudConflictModal).
const UNREACHABLE: ReadonlySet<string> = new Set(['cloud.network', 'cloud.account_unknown']);

export function cloudUnreachable(code: string): boolean {
  return UNREACHABLE.has(code);
}

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
