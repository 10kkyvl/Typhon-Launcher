import { msg } from '../i18n';
import type { LogUploadStatus } from '../services/logsUpload';

export function uploadPercent(status: LogUploadStatus): number {
  const total = status.totalBytes ?? 0;
  if (total <= 0) return 0;
  return Math.min(100, Math.round(((status.sentBytes ?? 0) / total) * 100));
}

export function logsStageText(status: LogUploadStatus | null): string {
  if (!status) return '';
  switch (status.state) {
    case 'preparing':
      return msg('settings.aboutLogsPreparingEllipsis');
    case 'sending':
      return (status.totalBytes ?? 0) > 0
        ? msg('settings.aboutLogsSendingProgress', { percent: uploadPercent(status) })
        : msg('settings.aboutLogsSendingEllipsis');
    case 'waiting':
      return msg('settings.aboutLogsWaitingEllipsis');
    default:
      return '';
  }
}
