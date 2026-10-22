import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { fetchPlayThrottle, idlePlayThrottle, type PlayThrottle } from '../services/playThrottle';
import { errorMessage } from '../utils/errors';
import { toast } from './toasts';

export const playThrottle = writable<PlayThrottle>(idlePlayThrottle);

export async function initPlayThrottle() {
  if (!inWails) return;
  let eventSeen = false;
  Events.On('downloads:play-throttle', (event) => {
    eventSeen = true;
    playThrottle.set(event.data as PlayThrottle);
  });
  try {
    const state = await fetchPlayThrottle();
    if (!eventSeen) playThrottle.set(state);
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}
