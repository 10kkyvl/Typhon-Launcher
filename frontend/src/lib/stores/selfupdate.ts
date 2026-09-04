import { derived, get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  acknowledgeReleaseNotes as acknowledgeReleaseNotesRequest,
  applyUpdate as applyUpdateRequest,
  cancelDownload as cancelDownloadRequest,
  checkForUpdate as checkForUpdateRequest,
  dismissUpdate as dismissUpdateRequest,
  downloadUpdate as downloadUpdateRequest,
  emptyReleaseNotes,
  getOutcome,
  getReleaseNotes as getReleaseNotesRequest,
  getStatus,
  type ReleaseNotes,
  type SelfUpdateOutcome,
  type SelfUpdateProgress,
  type SelfUpdateStatus,
} from '../services/selfupdate';
import { updateReason } from '../services/selfupdateMessages';
import { msg } from '../i18n';
import { toast } from './toasts';

export const selfUpdateStatus = writable<SelfUpdateStatus>({ state: 'idle', currentVersion: '' });
export const selfUpdateProgress = writable<SelfUpdateProgress | null>(null);
export const selfUpdateChecking = writable(false);
export const selfUpdateDownloading = writable(false);
export const selfUpdateLastAction = writable<'check' | 'download' | null>(null);
export const selfUpdateOutcome = writable<SelfUpdateOutcome | null>(null);
export const releaseNotes = writable<ReleaseNotes>(emptyReleaseNotes());

export const unseenReleaseNotes = derived(releaseNotes, ($notes) => $notes.unseen);
export const releaseNotesHistory = derived(releaseNotes, ($notes) => $notes.history);

// A downloaded installer stays installable whatever the last check said, and
// an available update stays downloadable: only its own failed download turns
// the banner into "failed".
export const selfUpdateView = derived(selfUpdateStatus, ($status) => {
  if ($status.state === 'failed') return 'failed';
  if (!$status.error) return $status.state;
  switch ($status.state) {
    case 'ready':
    case 'applying':
    case 'downloading':
      return $status.state;
    case 'available':
      return $status.errorCode === 'download' ? 'failed' : 'available';
    default:
      return 'failed';
  }
});

export const selfUpdateBusy = derived(
  [selfUpdateStatus, selfUpdateDownloading],
  ([$status, $downloading]) => $downloading || $status.state === 'downloading' || $status.state === 'applying',
);

export function dismissOutcome() {
  selfUpdateOutcome.set(null);
}

let announcedVersion = '';

function isCancelled(err: unknown): boolean {
  return typeof err === 'object' && err !== null && (err as { code?: string }).code === 'canceled';
}

function applyStatus(status: SelfUpdateStatus) {
  selfUpdateStatus.set(status);
  if (status.state !== 'downloading') {
    selfUpdateProgress.set(null);
  }
  if (!status.error && status.state === 'available' && status.availableVersion && status.availableVersion !== announcedVersion) {
    announcedVersion = status.availableVersion;
    toast(msg('state.selfupdateNewVersionToast', { version: status.availableVersion }));
  }
}

export async function initSelfUpdate() {
  try {
    applyStatus(await getStatus());
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
  try {
    releaseNotes.set(await getReleaseNotesRequest());
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
  try {
    const outcome = await getOutcome();
    if (outcome) {
      selfUpdateOutcome.set(outcome);
      // The "what's new" window is the notification for a successful update:
      // a toast on top of it says the same thing twice.
      const announced = outcome.ok && get(releaseNotes).unseen.length > 0;
      if (!announced) {
        toast(
          outcome.ok
            ? msg('state.selfupdateAppliedToast', { version: outcome.version })
            : msg('state.selfupdateApplyFailedToast', { version: outcome.version }),
          outcome.ok ? 'success' : 'danger',
        );
      }
    }
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
  if (!inWails) return () => {};

  const offStatus = Events.On('launcher:update_status', (event) => applyStatus(event.data as SelfUpdateStatus));
  const offProgress = Events.On('launcher:update_progress', (event) => selfUpdateProgress.set(event.data as SelfUpdateProgress));
  const offNotes = Events.On('launcher:release_notes', (event) => releaseNotes.set(event.data as ReleaseNotes));

  return () => {
    offStatus();
    offProgress();
    offNotes();
  };
}

export async function dismissReleaseNotes() {
  const current = get(releaseNotes);
  if (current.unseen.length === 0) return;
  releaseNotes.set({ ...current, unseen: [] });
  try {
    await acknowledgeReleaseNotesRequest();
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
}

export async function requestCheck() {
  selfUpdateLastAction.set('check');
  selfUpdateChecking.set(true);
  try {
    applyStatus(await checkForUpdateRequest());
  } catch (err) {
    if (!isCancelled(err)) toast(updateReason(err), 'danger');
  } finally {
    selfUpdateChecking.set(false);
  }
}

export async function requestDownload() {
  selfUpdateLastAction.set('download');
  selfUpdateProgress.set(null);
  selfUpdateDownloading.set(true);
  try {
    applyStatus(await downloadUpdateRequest());
  } catch (err) {
    if (!isCancelled(err)) toast(updateReason(err), 'danger');
  } finally {
    selfUpdateDownloading.set(false);
  }
}

export async function requestCancelDownload() {
  try {
    await cancelDownloadRequest();
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
}

export async function requestApply() {
  try {
    await applyUpdateRequest();
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
}

export async function requestDismiss() {
  try {
    await dismissUpdateRequest();
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
}

export function retryFailed() {
  const status = get(selfUpdateStatus);
  if (status.errorCode === 'download' || (!status.errorCode && get(selfUpdateLastAction) === 'download')) {
    return requestDownload();
  }
  return requestCheck();
}
