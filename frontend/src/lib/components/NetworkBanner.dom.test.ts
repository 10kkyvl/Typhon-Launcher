import { render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import NetworkBanner from './NetworkBanner.svelte';
import { locale } from '../i18n';
import { networkState } from '../stores/network';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

beforeEach(() => {
  locale.set('ru');
  networkState.set(null);
});

// The message goes into the alert after the alert is on the page, so it is waited for.
async function shown(): Promise<string> {
  const alert = screen.getByRole('alert');
  await waitFor(() => expect(alert.textContent?.trim()).not.toBe(''));
  return (alert.closest('.network-banner') as HTMLElement).textContent ?? '';
}

describe('NetworkBanner', () => {
  it('stays hidden while the route works or the mode is direct', () => {
    networkState.set({ mode: 'interface', state: 'ok', code: '', reason: '', address: '10.8.0.2', warning: '' });
    const { container, unmount } = render(NetworkBanner);
    expect(container.querySelector('.network-banner')).toBeNull();
    unmount();
    networkState.set({ mode: 'direct', state: 'down', code: 'download.no_client', reason: '', address: '', warning: '' });
    expect(render(NetworkBanner).container.querySelector('.network-banner')).toBeNull();
  });

  it('names the VPN and the reason when an adapter is down', async () => {
    networkState.set({ mode: 'interface', state: 'down', code: 'download.net_interface_down', reason: '', address: '', warning: '' });
    render(NetworkBanner);
    const text = await shown();
    expect(text).toContain('VPN отключён — загрузки и раздачи на паузе');
    expect(text).toContain('Сетевой адаптер отключён');
    expect(text).toContain('Настройки сети');
  });

  it('names the proxy when it is unreachable', async () => {
    networkState.set({ mode: 'proxy', state: 'down', code: 'download.proxy_unreachable', reason: '', address: '', warning: '' });
    render(NetworkBanner);
    const text = await shown();
    expect(text).toContain('Прокси недоступен — загрузки и раздачи на паузе');
    expect(text).toContain('Прокси недоступен');
  });
});
