import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { msg } from '../i18n';
import { inWails } from '../services/backend';
import { missingImages as fetchMissing } from '../services/emulators';
import { toast } from './toasts';

export const missingImages = writable<Set<string>>(new Set());

let failing = false;

export async function refreshMissingImages(): Promise<void> {
  try {
    missingImages.set(new Set(await fetchMissing()));
    failing = false;
  } catch (err) {
    console.error('missing images', err);
    if (!failing) toast(msg('emul.missingCheckFailed'), 'danger');
    failing = true;
  }
}

export function initMissingImages() {
  void refreshMissingImages();
  if (!inWails) return () => {};
  return Events.On('library:updated', () => {
    void refreshMissingImages();
  });
}
