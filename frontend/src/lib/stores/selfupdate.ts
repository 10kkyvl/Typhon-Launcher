import { derived, get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  applyUpdate as applyUpdateRequest,
  checkForUpdate as checkForUpdateRequest,
  dismissUpdate as dismissUpdateRequest,
  downloadUpdate as downloadUpdateRequest,
  getStatus,
  type SelfUpdateProgress,
  type SelfUpdateStatus,
} from '../services/selfupdate';
import { errorMessage } from '../utils/errors';
import { toast } from './toasts';

export const selfUpdateStatus = writable<SelfUpdateStatus>({ state: 'idle', currentVersion: '' });
export const selfUpdateProgress = writable<SelfUpdateProgress | null>(null);
export const selfUpdateChecking = writable(false);
export const selfUpdateDownloading = writable(false);
export const selfUpdateLastAction = writable<'check' | 'download' | null>(null);

export const selfUpdateView = derived(selfUpdateStatus, ($status) =>
  $status.error || $status.state === 'failed' ? 'failed' : $status.state,
);

let announcedVersion = '';

function applyStatus(status: SelfUpdateStatus) {
  selfUpdateStatus.set(status);
  if (!status.error && status.state === 'available' && status.availableVersion && status.availableVersion !== announcedVersion) {
    announcedVersion = status.availableVersion;
    toast(`Доступна новая версия лаунчера: ${status.availableVersion}`);
  }
}

export async function initSelfUpdate() {
  try {
    applyStatus(await getStatus());
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
  if (!inWails) return () => {};

  const offStatus = Events.On('launcher:update_status', (event) => applyStatus(event.data as SelfUpdateStatus));
  const offProgress = Events.On('launcher:update_progress', (event) => selfUpdateProgress.set(event.data as SelfUpdateProgress));

  return () => {
    offStatus();
    offProgress();
  };
}

export async function requestCheck() {
  selfUpdateLastAction.set('check');
  selfUpdateChecking.set(true);
  try {
    applyStatus(await checkForUpdateRequest());
  } catch (err) {
    toast(errorMessage(err), 'danger');
  } finally {
    selfUpdateChecking.set(false);
  }
}

export async function requestDownload() {
  selfUpdateLastAction.set('download');
  selfUpdateDownloading.set(true);
  try {
    applyStatus(await downloadUpdateRequest());
  } catch (err) {
    toast(errorMessage(err), 'danger');
  } finally {
    selfUpdateDownloading.set(false);
  }
}

export async function requestApply() {
  try {
    await applyUpdateRequest();
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}

export async function requestDismiss() {
  try {
    await dismissUpdateRequest();
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}

export function retryFailed() {
  if (get(selfUpdateLastAction) === 'download') {
    return requestDownload();
  }
  return requestCheck();
}
