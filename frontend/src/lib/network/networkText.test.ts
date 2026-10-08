import { describe, expect, it } from 'vitest';
import { networkChecking, networkCodeText, networkDownTitle, networkErrorText, networkIsDown, networkReasonText, networkWarningText } from './networkText';
import type { NetworkState } from '../services/network';

function state(patch: Partial<NetworkState>): NetworkState {
  return { mode: 'interface', state: 'down', code: '', reason: '', address: '', warning: '', ...patch };
}

describe('networkErrorText', () => {
  it('translates download codes', () => {
    expect(networkErrorText(new Error('typhon:download.proxy_auth_failed: rejected'))).toBe(
      'Прокси отклонил логин или пароль',
    );
  });

  it('translates settings validation codes', () => {
    expect(networkErrorText(new Error('typhon:settings.proxy_host_required'))).toBe('Не указан адрес прокси');
    expect(networkErrorText(new Error('typhon:settings.network_mode_invalid: "x"'))).toBe('Неизвестный режим сети');
  });

  it('never shows raw go error text', () => {
    const fallback = 'Не удалось выполнить действие с сетевыми настройками';
    expect(networkErrorText(new Error('dial tcp 10.0.0.1:1080: i/o timeout'))).toBe(fallback);
    expect(networkErrorText(new Error('typhon:settings.aboutTab'))).toBe(fallback);
    expect(networkErrorText(undefined)).toBe(fallback);
  });
});

describe('networkCodeText', () => {
  it('returns empty text for an unknown code', () => {
    expect(networkCodeText('download.something_new')).toBe('');
  });
});

describe('network state text', () => {
  it('reads the reason from the state code', () => {
    expect(networkReasonText(state({ code: 'download.net_interface_down', reason: 'raw' }))).toBe(
      'Сетевой адаптер отключён',
    );
    expect(networkReasonText(state({ code: 'download.no_client' }))).toBe('Торрент-клиент недоступен');
  });

  it('picks the banner title by mode', () => {
    expect(networkDownTitle(state({ mode: 'interface' }))).toBe('VPN отключён — загрузки и раздачи на паузе');
    expect(networkDownTitle(state({ mode: 'proxy' }))).toBe('Прокси недоступен — загрузки и раздачи на паузе');
  });

  it('treats only a non-direct down state as down', () => {
    expect(networkIsDown(null)).toBe(false);
    expect(networkIsDown(state({ mode: 'direct' }))).toBe(false);
    expect(networkIsDown(state({ mode: 'proxy', state: 'ok' }))).toBe(false);
    expect(networkIsDown(state({ mode: 'proxy' }))).toBe(true);
  });

  it('does not call a route that is being checked down', () => {
    const checking = state({ mode: 'proxy', code: 'download.network_checking' });
    expect(networkChecking(checking)).toBe(true);
    expect(networkIsDown(checking)).toBe(false);
    expect(networkChecking(state({ mode: 'proxy', code: 'download.network_down' }))).toBe(false);
    expect(networkChecking(state({ mode: 'proxy', state: 'ok', code: 'download.network_checking' }))).toBe(false);
    expect(networkChecking(null)).toBe(false);
  });
});

describe('networkWarningText', () => {
  it('explains a working adapter without DNS servers', () => {
    expect(networkWarningText(state({ state: 'ok', warning: 'download.net_interface_no_dns' }))).toMatch(/DNS/);
  });

  it('stays silent without a warning or while the network is down', () => {
    expect(networkWarningText(state({ state: 'ok' }))).toBe('');
    expect(networkWarningText(state({ state: 'down', warning: 'download.net_interface_no_dns' }))).toBe('');
    expect(networkWarningText(null)).toBe('');
  });
});
