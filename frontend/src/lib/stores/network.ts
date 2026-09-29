import { derived, get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { getNetworkStatus, type NetworkState } from '../services/network';
import { networkDownTitle, networkErrorText, networkIsDown } from '../network/networkText';
import { msg } from '../i18n';
import { toast } from './toasts';

export const networkState = writable<NetworkState | null>(null);

export const networkDown = derived(networkState, ($state) => networkIsDown($state));

function applyChange(next: NetworkState) {
  const previous = get(networkState);
  networkState.set(next);
  if (previous === null) return;
  const wasDown = networkIsDown(previous);
  const isDown = networkIsDown(next);
  if (isDown && !wasDown) toast(networkDownTitle(next), 'danger');
  if (!isDown && wasDown) toast(msg('ui.networkRestoredToast'), 'success');
}

export async function initNetwork() {
  let changed = false;
  if (inWails) {
    Events.On('download:network', (event) => {
      changed = true;
      applyChange(event.data as NetworkState);
    });
  }
  try {
    const initial = await getNetworkStatus();
    if (!changed) networkState.set(initial);
  } catch (err) {
    toast(networkErrorText(err), 'danger');
  }
}
