import { derived, get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  acknowledgeReleaseNotes as acknowledgeReleaseNotesRequest,
  applyRollback as applyRollbackRequest,
  applyUpdate as applyUpdateRequest,
  beginHandoff as beginHandoffRequest,
  cancelDownload as cancelDownloadRequest,
  checkForUpdate as checkForUpdateRequest,
  dismissUpdate as dismissUpdateRequest,
  downloadUpdate as downloadUpdateRequest,
  emptyReleaseNotes,
  toReleaseNotes,
  getOutcome,
  getReleaseNotes as getReleaseNotesRequest,
  getRollback,
  getStatus,
  installStable as installStableRequest,
  noRollback,
  type ReleaseNotes,
  type RollbackInfo,
  type SelfUpdateOutcome,
  type SelfUpdateProgress,
  type SelfUpdateStatus,
} from '../services/selfupdate';
import { updateReason } from '../services/selfupdateMessages';
import { msg } from '../i18n';
import { toast } from './toasts';
import { resumeFinishedShow, showFollowsStatus, updateShow, updateShowHandoff, type ConfirmedStart } from './updateShow';

export const selfUpdateStatus = writable<SelfUpdateStatus>({ state: 'idle', currentVersion: '' });

/**
 * A restart the player confirmed, from the press until Go answers it. Go says "applying" only after it
 * has checked the download once more; the press counts as applying already, so the show, the banner,
 * the sidebar and Big Picture hold still from the moment the player asked. `over` is Go's status when
 * the request was answered (null while it is in flight): any newer status is the truth again.
 */
interface Confirmed extends ConfirmedStart {
  over: SelfUpdateStatus | null;
}

const confirmed = writable<Confirmed | null>(null);
selfUpdateStatus.subscribe((status) => {
  const press = get(confirmed);
  if (press?.over && press.over !== status) confirmed.set(null);
});

const effectiveStatus = derived([selfUpdateStatus, confirmed], ([$status, $confirmed]): { status: SelfUpdateStatus; confirmed?: ConfirmedStart } => {
  if (!$confirmed || ($confirmed.over !== null && $confirmed.over !== $status)) return { status: $status };
  const start = { startedAt: $confirmed.startedAt, handoff: $confirmed.handoff };
  if ($status.state === 'applying') return { status: $status, confirmed: start };
  return { status: { ...$status, state: 'applying', error: undefined, errorCode: undefined }, confirmed: start };
});

// Every status the launcher holds, from Go or set by a test or the showcase, moves the update show.
effectiveStatus.subscribe(({ status, confirmed: start }) => showFollowsStatus(status, Date.now(), start));

/** The one answer to "is the launcher's update being applied?", the press of a restart included. */
export const selfUpdateApplying = derived(effectiveStatus, ({ status }) => status.state === 'applying');

/** Go rolled a seamless restart back: the launcher runs as before and the update is still ready. */
export const handoffFailed = derived(effectiveStatus, ({ status }) => status.state === 'ready' && status.errorCode === 'handoff_failed');
export const selfUpdateProgress = writable<SelfUpdateProgress | null>(null);
export const selfUpdateChecking = writable(false);
export const selfUpdateDownloading = writable(false);
export const selfUpdateLastAction = writable<'check' | 'download' | null>(null);
export const selfUpdateOutcome = writable<SelfUpdateOutcome | null>(null);
export const releaseNotes = writable<ReleaseNotes>(emptyReleaseNotes());
export const rollbackInfo = writable<RollbackInfo>(noRollback());
export const rollbackBusy = writable(false);
export const stableBusy = writable(false);

export const unseenReleaseNotes = derived(releaseNotes, ($notes) => $notes.unseen);
export const releaseNotesHistory = derived(releaseNotes, ($notes) => $notes.history);

// A downloaded installer stays installable whatever the last check said, and
// an available update stays downloadable: only its own failed download turns
// the banner into "failed".
export const selfUpdateView = derived(effectiveStatus, ({ status: $status }) => {
  if ($status.state === 'failed') return 'failed';
  if (!$status.error || $status.errorCode === 'rollback') return $status.state;
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
  [effectiveStatus, selfUpdateDownloading, rollbackBusy],
  ([{ status: $status }, $downloading, $rollbackBusy]) =>
    $downloading || $rollbackBusy || $status.state === 'downloading' || $status.state === 'applying',
);

export const OUTCOME_SUCCESS_MS = 2000;

let outcomeTimer: ReturnType<typeof setTimeout> | undefined;

export function dismissOutcome() {
  clearTimeout(outcomeTimer);
  selfUpdateOutcome.set(null);
}

function hideOutcomeSoon() {
  clearTimeout(outcomeTimer);
  outcomeTimer = setTimeout(dismissOutcome, OUTCOME_SUCCESS_MS);
}

// The card says the result once the update show has played its finish, then hides on its own.
function hideOutcomeAfterShow(): () => void {
  let waiting = true;
  let off = () => {};
  off = updateShow.subscribe((show) => {
    if (show || !waiting) return;
    waiting = false;
    hideOutcomeSoon();
    queueMicrotask(() => off());
  });
  return () => {
    waiting = false;
    off();
  };
}

/** A seamless restart leaves no worker outcome behind: the successor says the result itself. */
export function announceUpdated(version: string) {
  selfUpdateOutcome.set({ version, ok: true, finishedAt: new Date().toISOString() });
  hideOutcomeAfterShow();
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
  // Listen before asking. The backend runs its own check at startup and
  // answers with an event; landing between the first request and a
  // subscription made after it, that answer would be lost until the next
  // poll. An event that arrives while a request is in flight is also the
  // newer of the two, so the request's answer must not overwrite it.
  let statusFromEvent = false;
  let notesFromEvent = false;
  const offs: Array<() => void> = [];
  if (inWails) {
    offs.push(
      Events.On('launcher:update_status', (event) => {
        statusFromEvent = true;
        const status = event.data as SelfUpdateStatus;
        applyStatus(status);
        if (status.errorCode === 'rollback') {
          rollbackBusy.set(false);
          toast(msg('state.selfupdateRollbackMissedToast'), 'danger');
        }
      }),
      Events.On('launcher:update_progress', (event) => selfUpdateProgress.set(event.data as SelfUpdateProgress)),
      Events.On('launcher:release_notes', (event) => {
        notesFromEvent = true;
        releaseNotes.set(toReleaseNotes(event.data));
      }),
    );
  }
  try {
    const status = await getStatus();
    if (!statusFromEvent) applyStatus(status);
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
  try {
    const notes = await getReleaseNotesRequest();
    if (!notesFromEvent) releaseNotes.set(notes);
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
  try {
    const outcome = await getOutcome();
    if (outcome) {
      selfUpdateOutcome.set(outcome);
      // Read once the shell is up, the update is said by the card alone (resumeFinishedShow).
      if (outcome.ok && outcome.kind !== 'rolled_back' && resumeFinishedShow()) {
        offs.push(hideOutcomeAfterShow());
      } else if (outcome.ok) {
        hideOutcomeSoon();
      } else {
        const key = outcome.kind === 'rolled_back' ? 'state.selfupdateRollbackFailedToast' : 'state.selfupdateApplyFailedToast';
        toast(msg(key, { version: outcome.version }), 'danger');
      }
    }
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
  await refreshRollback();

  return () => {
    for (const off of offs) off();
  };
}

export async function refreshRollback() {
  try {
    rollbackInfo.set(await getRollback());
  } catch (err) {
    toast(updateReason(err), 'danger');
  }
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

/** False while an earlier press is still waiting for Go's answer. */
function confirmRestart(handoff: boolean): boolean {
  if (get(confirmed)?.over === null) return false;
  confirmed.set({ startedAt: Date.now(), handoff, over: null });
  return true;
}

function confirmedAnswered() {
  const status = get(selfUpdateStatus);
  confirmed.update((c) => c && { ...c, over: status });
}

async function applyConfirmed() {
  try {
    await applyUpdateRequest();
    confirmedAnswered();
  } catch (err) {
    confirmed.set(null);
    toast(updateReason(err), 'danger');
  }
}

/** The restart through the worker: the launcher closes and starts again on the new version. */
export async function requestApply() {
  if (!confirmRestart(false)) return;
  await applyConfirmed();
}

/**
 * The restart without a restart: the show starts at the press and Go gets that moment, so the
 * successor draws the same frame. Where Go refuses, the worker restarts the launcher at once.
 */
export async function requestHandoff() {
  if (!confirmRestart(true)) return;
  const startedAt = updateShowHandoff()?.startedAt ?? Date.now();
  try {
    await beginHandoffRequest(startedAt);
    confirmedAnswered();
    return;
  } catch (err) {
    // BeginHandoff's contract: an error means nothing changed and the worker path is open. Whatever
    // the cause (another platform, a portable build, a busy service), that path says its own error.
    // A platform that cannot hand over is the expected case and is not worth a log line.
    const unsupported = err instanceof Error && err.message.includes('selfupdate.handoff_unsupported');
    if (!unsupported) console.warn('seamless update refused, restarting instead', err);
  }
  confirmed.update((c) => c && { ...c, handoff: false });
  await applyConfirmed();
}

export async function requestRollback() {
  if (get(rollbackBusy)) return;
  rollbackBusy.set(true);
  try {
    await applyRollbackRequest();
  } catch (err) {
    rollbackBusy.set(false);
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

export async function requestInstallStable() {
  if (get(stableBusy)) return;
  stableBusy.set(true);
  try {
    applyStatus(await installStableRequest());
  } catch (err) {
    toast(updateReason(err), 'danger');
  } finally {
    stableBusy.set(false);
  }
}
