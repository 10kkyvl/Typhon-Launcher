export type RefreshStage = 'fetch' | 'parse' | 'match' | 'apply' | 'save';

export interface SourceProgress {
  stage: RefreshStage;
  done: number;
  total: number;
  startedAt: number;
  sawMatch: boolean;
}

export const stageOrder: readonly RefreshStage[] = ['fetch', 'parse', 'match', 'apply', 'save'];

export const stageWeights: Record<RefreshStage, number> = {
  fetch: 5,
  parse: 20,
  match: 60,
  apply: 10,
  save: 5,
};

export function isRefreshStage(value: unknown): value is RefreshStage {
  return typeof value === 'string' && (stageOrder as readonly string[]).includes(value);
}

export function stagePercent(stage: RefreshStage, done: number, total: number, hasMatch = true): number {
  const weights = hasMatch ? stageWeights : { ...stageWeights, match: 0 };
  const sum = stageOrder.reduce((acc, s) => acc + weights[s], 0);
  let before = 0;
  for (const s of stageOrder) {
    if (s === stage) break;
    before += weights[s];
  }
  const fraction = total > 0 ? Math.min(1, Math.max(0, done / total)) : 0;
  return ((before + weights[stage] * fraction) / sum) * 100;
}

export function progressPercent(progress: SourceProgress): number {
  const hasMatch = progress.sawMatch || stageOrder.indexOf(progress.stage) <= stageOrder.indexOf('match');
  return stagePercent(progress.stage, progress.done, progress.total, hasMatch);
}

export function overallPercent(progress: readonly SourceProgress[]): number {
  if (progress.length === 0) return 0;
  const sum = progress.reduce((acc, p) => acc + progressPercent(p), 0);
  return sum / progress.length;
}

export function isIndeterminate(progress: SourceProgress): boolean {
  return progress.total <= 0;
}
