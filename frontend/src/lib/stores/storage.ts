import { get, writable } from 'svelte/store';
import { getStorageInfo, type StorageInfo } from '../services/system';
import { settings } from './settings';

export const storageInfo = writable<StorageInfo | null>(null);

export async function refreshStorage() {
  if (!get(settings)?.libraryPath) {
    storageInfo.set(null);
    return;
  }
  try {
    storageInfo.set(await getStorageInfo());
  } catch (err) {
    storageInfo.set(null);
    console.error('storage info', err);
  }
}
