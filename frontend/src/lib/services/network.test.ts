import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = {
  NetworkStatus: vi.fn(),
  ListNetworkInterfaces: vi.fn(),
  HasProxyPassword: vi.fn(),
  SetProxyPassword: vi.fn(),
  TestProxy: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/download', () => ({ Manager: bindings }));

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./network');
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe('network interfaces', () => {
  it('gives every adapter an address list even when Go sent none', async () => {
    bindings.ListNetworkInterfaces.mockResolvedValue([
      { name: 'eth0', description: 'Ethernet', addresses: ['10.0.0.2'], up: true, vpnLike: false },
      { name: 'tun0', description: 'VPN', addresses: null, up: false, vpnLike: true },
    ]);
    const network = await load(true);

    const list = await network.listNetworkInterfaces();

    expect(list.map((item) => item.addresses)).toEqual([['10.0.0.2'], []]);
    expect(list[1]).toMatchObject({ name: 'tun0', up: false, vpnLike: true });
  });

  it('returns an empty list when Go sent none at all', async () => {
    bindings.ListNetworkInterfaces.mockResolvedValue(null);
    const network = await load(true);

    await expect(network.listNetworkInterfaces()).resolves.toEqual([]);
  });

  it('lets a listing failure reach the settings card, which has to show it', async () => {
    bindings.ListNetworkInterfaces.mockRejectedValue(new Error('typhon:download.net_interface_list_failed: x'));
    const network = await load(true);

    await expect(network.listNetworkInterfaces()).rejects.toThrow('net_interface_list_failed');
  });
});

describe('torrent network status', () => {
  it('hands over the status the backend reports, including a pending check', async () => {
    const state = { mode: 'interface', state: 'checking', code: 'download.network_checking', reason: '', address: '', warning: '' };
    bindings.NetworkStatus.mockResolvedValue(state);
    const network = await load(true);

    await expect(network.getNetworkStatus()).resolves.toEqual(state);
  });

  it('reports a healthy direct connection in a browser preview', async () => {
    const network = await load(false);

    await expect(network.getNetworkStatus()).resolves.toMatchObject({ mode: 'direct', state: 'ok' });
    expect(bindings.NetworkStatus).not.toHaveBeenCalled();
  });
});

describe('proxy password', () => {
  it('says there is none in a browser preview and refuses to store one', async () => {
    const network = await load(false);

    await expect(network.hasProxyPassword()).resolves.toBe(false);
    await expect(network.setProxyPassword('u', 'p')).rejects.toThrow('unavailable in browser');
    await expect(network.testProxy()).rejects.toThrow('unavailable in browser');
  });

  it('passes the login to the backend untouched', async () => {
    bindings.SetProxyPassword.mockResolvedValue(undefined);
    bindings.HasProxyPassword.mockResolvedValue(true);
    const network = await load(true);

    await network.setProxyPassword('user', 'secret');

    expect(bindings.SetProxyPassword).toHaveBeenCalledWith('user', 'secret');
    await expect(network.hasProxyPassword()).resolves.toBe(true);
  });

  it('lets a failed proxy test reach the card', async () => {
    bindings.TestProxy.mockRejectedValue(new Error('typhon:download.proxy_unreachable: x'));
    const network = await load(true);

    await expect(network.testProxy()).rejects.toThrow('proxy_unreachable');
  });
});
