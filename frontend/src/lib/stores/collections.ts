import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { getCollections, type LibraryCollection } from '../services/library';
import { libraryErrorText } from '../library/errors';
import { msg } from '../i18n';
import { toast } from './toasts';

export const collections = writable<LibraryCollection[]>([]);

export async function initCollections() {
  if (!inWails) return;
  let fromEvent = false;
  Events.On('library:collections', (event) => {
    fromEvent = true;
    collections.set((event.data as LibraryCollection[]) ?? []);
  });
  try {
    const loaded = await getCollections();
    if (!fromEvent) collections.set(loaded);
  } catch (err) {
    toast(libraryErrorText(err, msg('games.collectionsLoadFailed')), 'danger');
  }
}
