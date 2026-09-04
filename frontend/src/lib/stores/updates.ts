import { derived, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  buildManifest,
  cancelUpdate as cancelRequest,
  listUpdates,
  prepareUpdatePlan,
  repairGame,
  rollbackUpdate,
  startUpdate,
  verifyGame,
  type StepKind,
  type StrategyType,
  type Update,
  type UpdateState,
  type VerifyState,
} from '../services/updates';
import { errorMessage } from '../utils/errors';
import { msg } from '../i18n';
import { toast } from './toasts';

export const updates = writable<Update[]>([]);
export const verifications = writable<Record<string, VerifyState>>({});

export const updatesByGame = derived(updates, ($updates) => {
  const map = new Map<string, Update>();
  for (const u of $updates) map.set(u.gameId, u);
  return map;
});

export const updatesAvailable = derived(updates, ($updates) =>
  $updates.filter((u) => u.availability.available && u.state !== 'idle').length,
);

export function strategyLabels(strategy: StrategyType): string {
  const labels: Record<StrategyType, string> = {
    '': msg('state.updatesStrategyUnknown'),
    full_release: msg('state.updatesStrategyFullRelease'),
    torrent_reuse: msg('state.updatesStrategyTorrentReuse'),
    patch_chain: msg('state.updatesStrategyPatchChain'),
  };
  return labels[strategy];
}

export function stepLabels(step: StepKind): string {
  const labels: Record<StepKind, string> = {
    download: msg('common.loading'),
    recheck: msg('state.updatesStepRecheck'),
    apply_patch: msg('state.updatesStepApplyPatch'),
    extract: msg('state.updatesStepExtract'),
    install: msg('state.updatesStepInstall'),
    verify: msg('state.updatesStepVerify'),
    swap: msg('state.updatesStepSwap'),
    cleanup: msg('state.updatesStepCleanup'),
  };
  return labels[step];
}

export function updateStateLabels(state: UpdateState): string {
  const labels: Record<UpdateState, string> = {
    idle: msg('state.updatesStateIdle'),
    update_available: msg('state.updatesStateAvailable'),
    update_downloading: msg('state.updatesStateDownloading'),
    update_ready: msg('state.updatesStateReady'),
    updating: msg('state.updatesStateUpdating'),
    update_failed: msg('state.updatesStateFailed'),
    update_rollback: msg('state.updatesStateRollback'),
  };
  return labels[state];
}

function upsert(item: Update) {
  updates.update((list) => {
    const index = list.findIndex((u) => u.gameId === item.gameId);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

function upsertVerify(state: VerifyState) {
  verifications.update((map) => ({ ...map, [state.gameId]: state }));
}

export async function initUpdates() {
  updates.set(await listUpdates());
  if (!inWails) return;

  Events.On('update:updated', (event) => upsert(event.data as Update));
  Events.On('update:started', (event) => upsert(event.data as Update));
  Events.On('update:available', (event) => {
    const item = event.data as Update;
    upsert(item);
    toast(
      msg(
        item.availability.kind === 'update' ? 'state.updatesUpdateAvailableTitled' : 'state.updatesReleaseAvailableTitled',
        { title: item.title },
      ),
    );
  });
  Events.On('update:completed', (event) => {
    const item = event.data as Update;
    upsert(item);
    toast(msg('state.updatesUpdatedToast', { title: item.title }), 'success');
  });
  Events.On('update:failed', (event) => {
    const item = event.data as Update;
    upsert(item);
    toast(msg('state.updatesFailedToast', { title: item.title, error: item.error ?? '' }), 'danger');
  });
  Events.On('update:rollback', (event) => {
    const item = event.data as Update;
    upsert(item);
    toast(msg('state.updatesRollbackToast', { title: item.title }));
  });

  for (const name of ['verify:started', 'verify:updated', 'verify:completed', 'repair:started', 'repair:updated']) {
    Events.On(name, (event) => upsertVerify(event.data as VerifyState));
  }
  Events.On('repair:completed', (event) => {
    const state = event.data as VerifyState;
    upsertVerify(state);
    if (state.error) toast(msg('state.updatesRepairIncomplete', { error: state.error }), 'danger');
    else toast(msg('state.updatesRepairComplete'), 'success');
  });
}

async function run(action: () => Promise<void>) {
  try {
    await action();
  } catch (err) {
    toast(errorMessage(err), 'danger');
  }
}

export function preparePlan(gameId: string) {
  return run(() => prepareUpdatePlan(gameId));
}

export function applyUpdate(gameId: string) {
  return run(() => startUpdate(gameId));
}

export function abortUpdate(gameId: string) {
  return run(() => cancelRequest(gameId));
}

export function restorePrevious(gameId: string) {
  return run(() => rollbackUpdate(gameId));
}

export function verify(gameId: string) {
  return run(() => verifyGame(gameId));
}

export function repair(gameId: string) {
  return run(() => repairGame(gameId));
}

export function createManifest(gameId: string) {
  return run(() => buildManifest(gameId));
}
