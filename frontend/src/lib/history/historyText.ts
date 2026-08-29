import type { Record } from '../../../bindings/typhon/internal/history';
import { bytesSize } from '../utils/format';

export interface HistoryLabel {
  title: string;
  detail: string;
}

export function historyLabel(record: Record): HistoryLabel {
  const title = record.title || 'Игра';
  switch (record.kind) {
    case 'installed':
      return { title: `${title} установлен`, detail: '' };
    case 'updated':
      return {
        title: `${title} обновлён`,
        detail: record.fromVersion && record.toVersion ? `${record.fromVersion} → ${record.toVersion}` : '',
      };
    case 'install_failed':
      return { title: `${title} не установился`, detail: record.detail ?? '' };
    case 'update_failed':
      return { title: `${title} не обновился`, detail: record.detail ?? '' };
    case 'rolled_back':
      return { title: `${title} откачен к ${record.toVersion || '—'}`, detail: '' };
    case 'downloaded':
      return { title: `${title} загружен`, detail: '' };
    case 'uninstalled':
    case 'removed':
      return {
        title: `${title} удалён`,
        detail: record.bytesKnown ? `освобождено ${bytesSize(record.bytes ?? 0)}` : '',
      };
    case 'moved':
      return { title: `${title} перемещён`, detail: record.detail ?? '' };
    case 'lan_received':
      return { title: `${title} получен из локальной сети`, detail: '' };
    default:
      return { title: `${title}: событие в истории`, detail: record.detail ?? '' };
  }
}
