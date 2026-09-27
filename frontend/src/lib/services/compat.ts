import * as CompatService from '../../../bindings/typhon/internal/compat/service';
import { inWails } from './backend';

export type CompatState = 'unknown' | 'works' | 'broken';

export interface CompatStatus {
  gameId: string;
  state: CompatState;
  attempts: number;
  failures: number;
  bestSeconds: number;
  lastError?: string;
}

// Журнал ведётся локально и не обязан существовать: в браузерном предпросмотре
// и до первого запуска игры он просто пуст.
export async function compatStatuses(): Promise<Map<string, CompatStatus>> {
  if (!inWails) return new Map();
  const list = ((await CompatService.All()) ?? []) as CompatStatus[];
  return new Map(list.map((s) => [s.gameId, s]));
}
