import { render } from 'svelte/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import NetworkBanner from './NetworkBanner.svelte';
import { locale } from '../i18n';
import { networkState } from '../stores/network';

function body() {
  return render(NetworkBanner).body;
}

beforeEach(() => {
  locale.set('ru');
  networkState.set(null);
});

describe('NetworkBanner', () => {
  it('stays hidden while the route works or the mode is direct', () => {
    networkState.set({ mode: 'interface', state: 'ok', code: '', reason: '', address: '10.8.0.2', warning: '' });
    expect(body()).not.toContain('network-banner');
    networkState.set({ mode: 'direct', state: 'down', code: 'download.no_client', reason: '', address: '', warning: '' });
    expect(body()).not.toContain('network-banner');
  });

  it('names the VPN and the reason when an adapter is down', () => {
    networkState.set({ mode: 'interface', state: 'down', code: 'download.net_interface_down', reason: '', address: '', warning: '' });
    const html = body();
    expect(html).toContain('VPN отключён — загрузки и раздачи на паузе');
    expect(html).toContain('Сетевой адаптер отключён');
    expect(html).toContain('Настройки сети');
  });

  it('names the proxy when it is unreachable', () => {
    networkState.set({ mode: 'proxy', state: 'down', code: 'download.proxy_unreachable', reason: '', address: '', warning: '' });
    const html = body();
    expect(html).toContain('Прокси недоступен — загрузки и раздачи на паузе');
    expect(html).toContain('Прокси недоступен');
  });
});
