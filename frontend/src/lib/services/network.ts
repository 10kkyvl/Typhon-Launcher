import { Manager } from '../../../bindings/typhon/internal/download';
import { inWails } from './backend';

export type NetworkMode = 'direct' | 'interface' | 'proxy';

export interface NetworkState {
  mode: string;
  state: string;
  code: string;
  reason: string;
  address: string;
  warning: string;
}

export interface NetInterface {
  name: string;
  description: string;
  addresses: string[];
  up: boolean;
  vpnLike: boolean;
}

const unavailable = () => new Error('unavailable in browser');

export async function getNetworkStatus(): Promise<NetworkState> {
  if (!inWails) return { mode: 'direct', state: 'ok', code: '', reason: '', address: '', warning: '' };
  return (await Manager.NetworkStatus()) as NetworkState;
}

export async function listNetworkInterfaces(): Promise<NetInterface[]> {
  if (!inWails) return [];
  const list = await Manager.ListNetworkInterfaces();
  return (list ?? []).map((item) => ({ ...item, addresses: item.addresses ?? [] }));
}

export async function hasProxyPassword(): Promise<boolean> {
  if (!inWails) return false;
  return await Manager.HasProxyPassword();
}

export async function setProxyPassword(password: string): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.SetProxyPassword(password);
}

export async function testProxy(): Promise<void> {
  if (!inWails) throw unavailable();
  await Manager.TestProxy();
}
