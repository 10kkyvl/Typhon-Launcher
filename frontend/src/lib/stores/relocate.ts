import { derived, get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { moveErrorText } from '../relocate/moveMessages';
import { inWails } from '../services/backend';
import { listMoves, type MoveJob } from '../services/relocate';
import { libraryGames } from './library';
import { toast } from './toasts';

export const moves = writable<MoveJob[]>([]);

const terminalStages: MoveJob['stage'][] = ['done', 'failed', 'cancelled'];

export const activeMove = derived(
  moves,
  ($moves) => $moves.find((job) => !terminalStages.includes(job.stage)) ?? null,
);

export interface MoveTarget {
  gameId: string;
  title: string;
}

export const moveTarget = writable<MoveTarget | null>(null);

export function openMoveGame(gameId: string) {
  const game = get(libraryGames).find((g) => g.id === gameId);
  if (!game) return;
  moveTarget.set({ gameId, title: game.title });
}

export function closeMoveGame() {
  moveTarget.set(null);
}

function upsertMove(job: MoveJob) {
  moves.update((list) => {
    const index = list.findIndex((j) => j.id === job.id);
    if (index < 0) return [...list, job];
    const next = [...list];
    next[index] = job;
    return next;
  });
}

export async function initMoves() {
  moves.set(await listMoves());
  if (!inWails) return;

  Events.On('move:started', (event) => upsertMove(event.data as MoveJob));
  Events.On('move:progress', (event) => upsertMove(event.data as MoveJob));
  Events.On('move:completed', (event) => {
    const job = event.data as MoveJob;
    upsertMove(job);
    toast(`«${job.title || 'Библиотека'}» перенесена`, 'success');
  });
  Events.On('move:failed', (event) => {
    const job = event.data as MoveJob;
    upsertMove(job);
    toast(moveErrorText(job.error, `Не удалось перенести «${job.title || 'библиотеку'}»`), 'danger');
  });
  Events.On('move:cancelled', (event) => upsertMove(event.data as MoveJob));
}
