import type { MoveJob, MoveStage } from '../services/relocate';
import { bytesSize } from '../utils/format';

const STAGE_LABELS: Record<MoveStage, string> = {
  prepare: 'Подготовка',
  copy: 'Копирование',
  verify: 'Проверка',
  commit: 'Завершение',
  repoint: 'Перепривязка',
  cleanup: 'Уборка',
  done: 'Готово',
  failed: 'Не удалось',
  cancelled: 'Отменено',
};

export function stageLabel(stage: MoveStage): string {
  return STAGE_LABELS[stage] ?? stage;
}

export function movePercent(job: Pick<MoveJob, 'copiedBytes' | 'totalBytes'>): number {
  if (job.totalBytes <= 0) return 0;
  return Math.min(100, Math.max(0, Math.round((job.copiedBytes / job.totalBytes) * 100)));
}

export function moveSummary(job: Pick<MoveJob, 'stage' | 'phase' | 'copiedBytes' | 'totalBytes'>): string {
  const stage = stageLabel(job.stage);
  if (job.stage === 'copy' && job.totalBytes > 0) {
    return `${stage} ${movePercent(job)}% · ${bytesSize(job.copiedBytes)} из ${bytesSize(job.totalBytes)}`;
  }
  if (job.phase) return `${stage}: ${job.phase}`;
  return stage;
}
