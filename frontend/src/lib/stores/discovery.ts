import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  isScanning,
  scanInstalledGames,
  type DiscoveryProgress,
  type DiscoveryResult,
} from '../services/discovery';
import { msg } from '../i18n';

export const scanning = writable(false);
export const scanProgress = writable<DiscoveryProgress>({ processed: 0, total: 0 });
export const lastScan = writable<DiscoveryResult | null>(null);

export async function initDiscovery() {
  if (!inWails) return;
  scanning.set(await isScanning());

  Events.On('discovery:started', (event) => {
    const data = event.data as DiscoveryProgress | undefined;
    scanning.set(true);
    scanProgress.set({ processed: 0, total: data?.total ?? 0 });
  });
  Events.On('discovery:progress', (event) => {
    const data = event.data as DiscoveryProgress | undefined;
    if (data) scanProgress.set(data);
  });
  Events.On('discovery:completed', (event) => {
    scanning.set(false);
    lastScan.set((event.data as DiscoveryResult) ?? null);
  });
}

export async function rescan(): Promise<DiscoveryResult> {
  scanning.set(true);
  scanProgress.set({ processed: 0, total: 0 });
  try {
    return await scanInstalledGames();
  } finally {
    scanning.set(false);
  }
}

export function scanSummary(result: DiscoveryResult): string {
  const parts = [msg('state.discoveryFound', { count: result.candidates })];
  if (result.added > 0) parts.push(msg('state.discoveryAdded', { count: result.added }));
  if (result.updated > 0) parts.push(msg('state.discoveryUpdated', { count: result.updated }));
  if (result.known > 0) parts.push(msg('state.discoveryKnown', { count: result.known }));
  if (result.skipped > 0) parts.push(msg('state.discoverySkipped', { count: result.skipped }));
  if (result.errors > 0) parts.push(msg('state.discoveryErrors', { count: result.errors }));
  return parts.join(' · ');
}
