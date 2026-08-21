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

export const strategyLabels: Record<StrategyType, string> = {
  '': 'Не определена',
  full_release: 'Полная переустановка',
  torrent_reuse: 'Повторное использование файлов',
  patch_chain: 'Патчи',
};

export const stepLabels: Record<StepKind, string> = {
  download: 'Загрузка',
  recheck: 'Проверка файлов',
  apply_patch: 'Применение патча',
  extract: 'Распаковка',
  install: 'Установка',
  verify: 'Проверка',
  swap: 'Замена версии',
  cleanup: 'Очистка',
};

export const updateStateLabels: Record<UpdateState, string> = {
  idle: 'Актуальная версия',
  update_available: 'Доступно обновление',
  update_downloading: 'Загрузка обновления',
  update_ready: 'Обновление готово',
  updating: 'Обновление',
  update_failed: 'Ошибка обновления',
  update_rollback: 'Откат',
};

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
    const label = item.availability.kind === 'update' ? 'Доступно обновление' : 'Доступен новый релиз';
    toast(`${label}: «${item.title}»`);
  });
  Events.On('update:completed', (event) => {
    const item = event.data as Update;
    upsert(item);
    toast(`«${item.title}» обновлена`, 'success');
  });
  Events.On('update:failed', (event) => {
    const item = event.data as Update;
    upsert(item);
    toast(`Не удалось обновить «${item.title}»: ${item.error}`, 'danger');
  });
  Events.On('update:rollback', (event) => {
    const item = event.data as Update;
    upsert(item);
    toast(`Восстановлена предыдущая версия «${item.title}»`);
  });

  for (const name of ['verify:started', 'verify:updated', 'verify:completed', 'repair:started', 'repair:updated']) {
    Events.On(name, (event) => upsertVerify(event.data as VerifyState));
  }
  Events.On('repair:completed', (event) => {
    const state = event.data as VerifyState;
    upsertVerify(state);
    if (state.error) toast(`Восстановление не завершено: ${state.error}`, 'danger');
    else toast('Файлы игры восстановлены', 'success');
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
