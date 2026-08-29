import { bytesSize } from '../utils/format';
import type { Offer, Stats, Transfer } from '../services/lan';

const rejectedLabels: Record<string, string> = {
  too_large: 'слишком большой пакет',
  bad_source_addr: 'адрес отправителя не локальный',
  bad_json: 'повреждённые данные',
  bad_version: 'неподдерживаемая версия протокола',
  bad_id: 'некорректный идентификатор',
  own_id: 'от самого себя',
  bad_host: 'некорректное имя хоста',
  bad_port: 'некорректный порт',
  bad_infohash: 'некорректный infohash',
  bad_title: 'некорректное название',
  bad_version_field: 'некорректная версия игры',
  bad_gameid: 'некорректный id игры',
  bad_size: 'некорректный размер',
  bad_exe: 'некорректный путь к исполняемому файлу',
  bad_ts: 'метка времени вне диапазона',
  capacity: 'превышена ёмкость таблицы',
  rate_limited: 'слишком частые сообщения',
  unknown: 'неизвестная причина',
};

export function transferLabel(transfer: Transfer): string {
  switch (transfer.status) {
    case 'receiving': {
      if (transfer.total <= 0) return 'Получение…';
      const pct = Math.round((transfer.downloaded / transfer.total) * 100);
      return `Получение… ${pct}%`;
    }
    case 'completed':
      return 'Получено';
    case 'failed':
      return `Не удалось: ${transfer.error || 'неизвестная ошибка'}`;
    case 'cancelled':
      return 'Отменено';
    default:
      return '';
  }
}

export function offerLabel(offer: Offer): string {
  const parts = [offer.title];
  if (offer.version) parts.push(offer.version);
  parts.push(bytesSize(offer.sizeBytes));
  parts.push(`с ПК ${offer.host}`);
  return parts.join(' · ');
}

export function rejectedSummary(stats: Stats): string {
  if (!stats.rejected) return '';
  const entries = Object.entries(stats.rejected).filter(([, count]) => (count ?? 0) > 0);
  if (entries.length === 0) return '';
  entries.sort((a, b) => (b[1] ?? 0) - (a[1] ?? 0));
  return entries.map(([key, count]) => `${rejectedLabels[key] ?? key}: ${count}`).join(' · ');
}
