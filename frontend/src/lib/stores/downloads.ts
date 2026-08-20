import { derived, writable } from 'svelte/store';
import { initialActive, initialQueue } from '../mock/downloads';
import { gameById } from '../mock/games';
import type { ActiveDownload, DownloadStage, QueuedDownload } from '../mock/types';
import { toast } from './toasts';

export const active = writable<ActiveDownload[]>(structuredClone(initialActive));
export const queue = writable<QueuedDownload[]>(structuredClone(initialQueue));

export const stats = derived(active, ($active) => {
  const running = $active.filter((d) => !d.paused && d.stage === 'downloading');
  const down = running.reduce((sum, d) => sum + d.speedMbs, 0);
  return {
    downSpeed: down,
    upSpeed: down > 0 ? Math.round(down * 0.11 * 10) / 10 : 0,
    activeCount: $active.length,
  };
});

const stageOrder: DownloadStage[] = ['downloading', 'unpacking', 'verifying', 'installing', 'complete'];

export const stageLabels: Record<DownloadStage, string> = {
  downloading: 'Загрузка',
  unpacking: 'Распаковка',
  verifying: 'Проверка',
  installing: 'Установка',
  complete: 'Готово',
};

function jitter(base: number) {
  return Math.max(4, base * (0.85 + Math.random() * 0.3));
}

function promoteFromQueue() {
  queue.update((q) => {
    const [next, ...rest] = q;
    if (next) {
      const game = gameById(next.gameId);
      active.update((a) => [
        ...a,
        {
          id: `dl-${next.id}`,
          gameId: next.gameId,
          label: 'Базовая игра',
          doneGb: 0,
          totalGb: next.sizeGb,
          speedMbs: 24,
          baseMbs: 24,
          peers: [Math.floor(12 + Math.random() * 40), Math.floor(60 + Math.random() * 40)],
          stage: 'downloading',
          stagePct: 0,
          paused: false,
        },
      ]);
      if (game) toast(`Загрузка «${game.title}» началась`);
    }
    return rest;
  });
}

function tick() {
  let completed: ActiveDownload[] = [];
  active.update((list) => {
    const next = list.map((d) => {
      if (d.paused || d.stage === 'complete') return d;
      if (d.stage === 'downloading') {
        const speed = jitter(d.baseMbs);
        const doneGb = Math.min(d.totalGb, d.doneGb + speed / 1024);
        if (doneGb >= d.totalGb) {
          return { ...d, doneGb: d.totalGb, speedMbs: 0, stage: 'unpacking' as DownloadStage, stagePct: 0 };
        }
        return { ...d, doneGb, speedMbs: Math.round(speed * 10) / 10 };
      }
      const stagePct = d.stagePct + 0.08 + Math.random() * 0.06;
      if (stagePct >= 1) {
        const idx = stageOrder.indexOf(d.stage);
        const stage = stageOrder[idx + 1];
        return { ...d, stage, stagePct: 0 };
      }
      return { ...d, stagePct };
    });
    completed = next.filter((d) => d.stage === 'complete');
    return next.filter((d) => d.stage !== 'complete');
  });
  for (const d of completed) {
    const game = gameById(d.gameId);
    toast(`«${game?.title ?? d.label}» установлена`, 'success');
    promoteFromQueue();
  }
}

let timer: ReturnType<typeof setInterval> | undefined;

export function startDownloadSim() {
  if (timer) return;
  timer = setInterval(tick, 1000);
}

export function togglePause(id: string) {
  active.update((list) => list.map((d) => (d.id === id ? { ...d, paused: !d.paused } : d)));
}

export function cancelDownload(id: string) {
  active.update((list) => list.filter((d) => d.id !== id));
}

export function removeFromQueue(id: string) {
  queue.update((q) => q.filter((d) => d.id !== id));
}

export function moveInQueue(id: string, dir: -1 | 1) {
  queue.update((q) => {
    const idx = q.findIndex((d) => d.id === id);
    const target = idx + dir;
    if (idx < 0 || target < 0 || target >= q.length) return q;
    const next = [...q];
    [next[idx], next[target]] = [next[target], next[idx]];
    return next;
  });
}
