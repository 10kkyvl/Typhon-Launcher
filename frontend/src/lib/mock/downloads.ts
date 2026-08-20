import type { ActiveDownload, QueuedDownload } from './types';

export const initialActive: ActiveDownload[] = [
  {
    id: 'dl-elden-ring',
    gameId: 'elden-ring',
    label: 'Базовая игра',
    doneGb: 32.6,
    totalGb: 60.0,
    speedMbs: 28.4,
    baseMbs: 28.4,
    peers: [42, 85],
    stage: 'downloading',
    stagePct: 0,
    paused: false,
  },
  {
    id: 'dl-hogwarts',
    gameId: 'hogwarts-legacy',
    label: 'Обновление 1.0.3.0',
    doneGb: 8.2,
    totalGb: 15.7,
    speedMbs: 20.3,
    baseMbs: 20.3,
    peers: [31, 67],
    stage: 'downloading',
    stagePct: 0,
    paused: false,
  },
];

export const initialQueue: QueuedDownload[] = [
  { id: 'q-jedi', gameId: 'jedi-survivor', sizeGb: 78.4 },
  { id: 'q-rdr2', gameId: 'rdr2', sizeGb: 97 },
  { id: 'q-gow', gameId: 'god-of-war', sizeGb: 84.7 },
  { id: 'q-tsushima', gameId: 'ghost-of-tsushima', sizeGb: 75.2 },
];
