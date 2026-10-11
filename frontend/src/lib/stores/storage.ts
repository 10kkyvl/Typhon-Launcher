import { get, writable } from 'svelte/store';
import { getStorageInfo, type StorageInfo } from '../services/system';
import { settings } from './settings';

export const storageInfo = writable<StorageInfo | null>(null);
export const storageFailed = writable(false);

let seq = 0;

export async function refreshStorage() {
  const mine = ++seq;
  if (!get(settings)?.libraryPath) {
    storageInfo.set(null);
    storageFailed.set(false);
    return;
  }
  try {
    const info = await getStorageInfo();
    if (mine !== seq) return;
    storageInfo.set(info);
    storageFailed.set(false);
  } catch {
    if (mine !== seq) return;
    storageInfo.set(null);
    storageFailed.set(true);
  }
}
