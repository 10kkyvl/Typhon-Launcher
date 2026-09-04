import { msg } from '../i18n';
import type { MoveJob, MoveStage } from '../services/relocate';
import { bytesSize } from '../utils/format';

export function stageLabel(stage: MoveStage): string {
  switch (stage) {
    case 'prepare':
      return msg('transfers.moveStagePrepare');
    case 'copy':
      return msg('transfers.moveStageCopy');
    case 'verify':
      return msg('transfers.moveStageVerify');
    case 'commit':
      return msg('transfers.moveStageCommit');
    case 'repoint':
      return msg('transfers.moveStageRepoint');
    case 'cleanup':
      return msg('transfers.moveStageCleanup');
    case 'done':
      return msg('common.done');
    case 'failed':
      return msg('transfers.moveStageFailed');
    case 'cancelled':
      return msg('transfers.moveStageCancelled');
    default:
      return stage;
  }
}

export function movePercent(job: Pick<MoveJob, 'copiedBytes' | 'totalBytes'>): number {
  if (job.totalBytes <= 0) return 0;
  return Math.min(100, Math.max(0, Math.round((job.copiedBytes / job.totalBytes) * 100)));
}

export function moveSummary(job: Pick<MoveJob, 'stage' | 'phase' | 'copiedBytes' | 'totalBytes'>): string {
  const stage = stageLabel(job.stage);
  if (job.stage === 'copy' && job.totalBytes > 0) {
    return msg('transfers.moveCopyProgress', {
      stage,
      percent: movePercent(job),
      copied: bytesSize(job.copiedBytes),
      total: bytesSize(job.totalBytes),
    });
  }
  if (job.phase) return msg('transfers.movePhaseSummary', { stage, phase: job.phase });
  return stage;
}
