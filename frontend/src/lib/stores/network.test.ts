import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import type { NetworkState } from '../services/network';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return vi.fn();
    }),
  },
}));

vi.mock('../services/backend', () => ({ inWails: true }));

vi.mock('./toasts', () => ({ toast: vi.fn() }));

let initial: NetworkState | Error;

vi.mock('../services/network', () => ({
  getNetworkStatus: vi.fn(async () => {
    if (initial instanceof Error) throw initial;
    return initial;
  }),
}));

function makeState(patch: Partial<NetworkState> = {}): NetworkState {
  return { mode: 'interface', state: 'ok', code: '', reason: '', address: '10.8.0.2', warning: '', ...patch };
}

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const store = await import('./network');
  const { toast } = await import('./toasts');
  await store.initNetwork();
  return { store, toast: vi.mocked(toast) };
}

beforeEach(() => {
  vi.clearAllMocks();
  initial = makeState();
});

describe('network store', () => {
  it('loads the initial state without a toast', async () => {
    initial = makeState({ state: 'down', code: 'download.net_interface_down' });
    const { store, toast } = await load();
    expect(get(store.networkState)?.state).toBe('down');
    expect(get(store.networkDown)).toBe(true);
    expect(toast).not.toHaveBeenCalled();
  });

  it('toasts once when the route goes down and once when it returns', async () => {
    const { store, toast } = await load();
    handlers['download:network']({ data: makeState({ state: 'down', code: 'download.net_interface_down' }) });
    expect(get(store.networkDown)).toBe(true);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast).toHaveBeenLastCalledWith('VPN отключён — загрузки и раздачи на паузе', 'danger');

    handlers['download:network']({ data: makeState({ state: 'down', code: 'download.net_interface_missing' }) });
    expect(toast).toHaveBeenCalledTimes(1);

    handlers['download:network']({ data: makeState() });
    expect(get(store.networkDown)).toBe(false);
    expect(toast).toHaveBeenCalledTimes(2);
    expect(toast).toHaveBeenLastCalledWith('Сеть для торрентов снова доступна — загрузки продолжатся', 'success');
  });

  it('stays quiet while a working route is replaced', async () => {
    const { store, toast } = await load();
    handlers['download:network']({ data: makeState({ state: 'down', code: 'download.network_checking', address: '' }) });
    expect(get(store.networkDown)).toBe(false);
    handlers['download:network']({ data: makeState({ address: '10.8.0.3' }) });
    expect(toast).not.toHaveBeenCalled();
  });

  it('remembers a down route across the check that follows it', async () => {
    const { toast } = await load();
    handlers['download:network']({ data: makeState({ state: 'down', code: 'download.net_interface_down' }) });
    handlers['download:network']({ data: makeState({ state: 'down', code: 'download.network_checking' }) });
    expect(toast).toHaveBeenCalledTimes(1);
    handlers['download:network']({ data: makeState({ state: 'down', code: 'download.net_interface_missing' }) });
    expect(toast).toHaveBeenCalledTimes(1);
    handlers['download:network']({ data: makeState({ state: 'down', code: 'download.network_checking' }) });
    handlers['download:network']({ data: makeState() });
    expect(toast).toHaveBeenCalledTimes(2);
    expect(toast).toHaveBeenLastCalledWith('Сеть для торрентов снова доступна — загрузки продолжатся', 'success');
  });

  it('does not raise the banner in direct mode', async () => {
    initial = makeState({ mode: 'direct', address: '' });
    const { store, toast } = await load();
    handlers['download:network']({ data: makeState({ mode: 'direct', state: 'down', code: 'download.no_client' }) });
    expect(get(store.networkDown)).toBe(false);
    expect(toast).not.toHaveBeenCalled();
  });

  it('shows the failure when the initial status cannot be read', async () => {
    initial = new Error('typhon:download.unavailable');
    const { store, toast } = await load();
    expect(get(store.networkState)).toBeNull();
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });
});
