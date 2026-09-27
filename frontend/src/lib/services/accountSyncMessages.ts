import { msg } from '../i18n';

function reasons(): [string, string][] {
  return [
    ['not authenticated', msg('state.accountSyncReasonUnauthenticated')],
    ['settings revision conflict', msg('state.accountSyncReasonRevisionConflict')],
    ['too many games in request', msg('state.accountSyncReasonTooManyGames')],
    ['device limit reached', msg('state.accountSyncReasonDeviceLimit')],
    ['sync already in progress', msg('state.accountSyncReasonInProgress')],
    ['service is not started', msg('state.accountSyncReasonNotStarted')],
    ['network error', msg('state.accountSyncReasonNetworkError')],
    ['server error, status', msg('state.accountSyncReasonServerError')],
    ['request rejected', msg('state.accountSyncReasonRequestRejected')],
    ['unavailable in browser', msg('state.accountSyncReasonUnavailableInBrowser')],
  ];
}

function translate(raw: string): string {
  const known = reasons().find(([marker]) => raw.includes(marker));
  return known ? known[1] : '';
}

export function accountSyncReason(err: unknown, fallback: string): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  return translate(raw) || fallback;
}
