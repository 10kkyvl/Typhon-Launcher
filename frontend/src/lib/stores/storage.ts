import { get, writable } from 'svelte/store';
import { getStorageInfo, type StorageInfo } from '../services/system';
import { settings } from './settings';

export const storageInfo = writable<StorageInfo | null>(null);
export const storageFailed = writable(false);

export async function refreshStorage() {
  if (!get(settings)?.libraryPath) {
    storageInfo.set(null);
    storageFailed.set(false);
    return;
  }
  try {
    storageInfo.set(await getStorageInfo());
    storageFailed.set(false);
  } catch {
    storageInfo.set(null);
    storageFailed.set(true);
  }
}
