import { writable } from 'svelte/store';
import { inWails } from '../services/backend';
import { getCollections, onCollectionsChanged, type LibraryCollection } from '../services/library';

/** 'failed': the list could not be read; it is not the same as having no collections. */
export type CollectionsState = 'loading' | 'ready' | 'failed';

export const collections = writable<LibraryCollection[]>([]);
export const collectionsState = writable<CollectionsState>('loading');
export const collectionsLoadError = writable<unknown>(null);

// An event carries the newest list; an answer that started before it must not replace it.
let generation = 0;

export async function loadCollections(): Promise<boolean> {
  const started = generation;
  try {
    const loaded = await getCollections();
    if (started === generation) {
      collections.set(loaded);
      collectionsState.set('ready');
      collectionsLoadError.set(null);
    }
    return true;
  } catch (err) {
    if (started !== generation) return true;
    collectionsState.set('failed');
    collectionsLoadError.set(err);
    return false;
  }
}

/** Subscribes first, then loads; never rejects. The answer is the disposer of the subscription. */
export async function initCollections(): Promise<() => void> {
  if (!inWails) {
    collectionsState.set('ready');
    return () => {};
  }
  const off = onCollectionsChanged((list) => {
    generation += 1;
    collections.set(list);
    collectionsState.set('ready');
    collectionsLoadError.set(null);
  });
  await loadCollections();
  return off;
}
