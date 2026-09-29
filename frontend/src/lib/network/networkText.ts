import { errorCode, hasMessage, msg } from '../i18n';
import { REASONS } from '../install/installErrors';
import type { NetworkState } from '../services/network';

function capitalized(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function rawCodeText(code: string): string {
  const key = REASONS[code];
  if (key) return msg(key);
  if (code.startsWith('settings.') && hasMessage(code)) return msg(code);
  return '';
}

export function networkCodeText(code: string): string {
  const text = rawCodeText(code);
  return text ? capitalized(text) : '';
}

export function networkErrorText(err: unknown, fallback: string = msg('settings.networkFallbackError')): string {
  return networkCodeText(errorCode(err)) || fallback;
}

export function networkReasonText(state: NetworkState): string {
  return networkCodeText(state.code);
}

export function networkWarningText(state: NetworkState | null): string {
  return state && state.state === 'ok' && state.warning ? networkCodeText(state.warning) : '';
}

export function networkIsDown(state: NetworkState | null): boolean {
  return state !== null && state.mode !== 'direct' && state.state === 'down';
}

export function networkDownTitle(state: NetworkState): string {
  return state.mode === 'proxy' ? msg('ui.networkBannerProxy') : msg('ui.networkBannerInterface');
}
