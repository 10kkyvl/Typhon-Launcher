import { msg } from '../i18n';
import type { Record } from '../../../bindings/typhon/internal/history';
import { bytesSize } from '../utils/format';

export interface HistoryLabel {
  title: string;
  detail: string;
}

export function historyLabel(record: Record): HistoryLabel {
  const title = record.title || msg('transfers.historyUntitledGame');
  switch (record.kind) {
    case 'installed':
      return { title: msg('transfers.historyInstalledTitle', { title }), detail: '' };
    case 'updated':
      return {
        title: msg('transfers.historyUpdatedTitle', { title }),
        detail:
          record.fromVersion && record.toVersion
            ? msg('transfers.historyVersionJump', { from: record.fromVersion, to: record.toVersion })
            : '',
      };
    case 'install_failed':
      return { title: msg('transfers.historyInstallFailedTitle', { title }), detail: record.detail ?? '' };
    case 'update_failed':
      return { title: msg('transfers.historyUpdateFailedTitle', { title }), detail: record.detail ?? '' };
    case 'rolled_back':
      return {
        title: msg('transfers.historyRolledBackTitle', { title, version: record.toVersion || '—' }),
        detail: '',
      };
    case 'downloaded':
      return { title: msg('transfers.historyDownloadedTitle', { title }), detail: '' };
    case 'uninstalled':
    case 'removed':
      return {
        title: msg('transfers.historyRemovedTitle', { title }),
        detail: record.bytesKnown ? msg('transfers.historyFreedSize', { size: bytesSize(record.bytes ?? 0) }) : '',
      };
    case 'moved':
      return { title: msg('transfers.historyMovedTitle', { title }), detail: record.detail ?? '' };
    case 'lan_received':
      return { title: msg('transfers.historyLanReceivedTitle', { title }), detail: '' };
    default:
      return { title: msg('transfers.historyUnknownTitle', { title }), detail: record.detail ?? '' };
  }
}
