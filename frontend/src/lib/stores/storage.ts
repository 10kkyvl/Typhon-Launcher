import { writable } from 'svelte/store';
import { getStorageInfo, type StorageInfo } from '../services/system';

export const storageInfo = writable<StorageInfo | null>(null);

export async function refreshStorage() {
  try {
    storageInfo.set(await getStorageInfo());
  } catch (err) {
    console.error('storage info', err);
  }
}
