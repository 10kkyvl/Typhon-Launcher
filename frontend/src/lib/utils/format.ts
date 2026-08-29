export function gb(value: number, digits = 1) {
  return `${value.toFixed(digits).replace('.', ',')} ГБ`;
}

export function speed(mbs: number) {
  return `${mbs.toFixed(1).replace('.', ',')} МБ/с`;
}

export function eta(doneGb: number, totalGb: number, speedMbs: number) {
  if (speedMbs <= 0) return '—';
  const seconds = ((totalGb - doneGb) * 1024) / speedMbs;
  const min = Math.floor(seconds / 60);
  const sec = Math.floor(seconds % 60);
  if (min >= 60) {
    const h = Math.floor(min / 60);
    return `${h} ч ${min % 60} мин`;
  }
  return `${min} мин ${sec} сек`;
}

export function plural(n: number, one: string, few: string, many: string) {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}

export function formatCount(n: number) {
  return n.toLocaleString('ru-RU');
}

export function playtime(seconds: number) {
  if (seconds < 60) return seconds > 0 ? 'меньше минуты' : '—';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours === 0) return `${minutes} мин`;
  if (hours < 10 && minutes > 0) return `${hours} ч ${minutes} мин`;
  return `${hours} ч`;
}

export function relativeDate(iso: string | null) {
  if (!iso) return '—';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '—';
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((startOfDay(new Date()) - startOfDay(date)) / 86400000);
  if (days <= 0) return 'Сегодня';
  if (days === 1) return 'Вчера';
  if (days < 7) return `${days} ${plural(days, 'день', 'дня', 'дней')} назад`;
  if (days < 30) {
    const weeks = Math.round(days / 7);
    return `${weeks} ${plural(weeks, 'неделю', 'недели', 'недель')} назад`;
  }
  return date.toLocaleDateString('ru-RU');
}

const GB_BYTES = 1024 ** 3;

export function bytesToGb(bytes: number) {
  return bytes / GB_BYTES;
}

export function bytesLabel(bytes: number) {
  const gbValue = bytes / GB_BYTES;
  if (gbValue >= 1000) {
    const tb = gbValue / 1024;
    const text = (Math.round(tb * 10) / 10).toString().replace('.', ',');
    return `${text} ТБ`;
  }
  return `${Math.round(gbValue)} ГБ`;
}

const KB_BYTES = 1024;
const MB_BYTES = 1024 ** 2;
const TB_BYTES = 1024 ** 4;

function decimal(value: number, digits: number) {
  return value.toFixed(digits).replace('.', ',');
}

export function bytesSize(bytes: number) {
  const value = Math.max(0, bytes);
  if (value >= TB_BYTES) return `${decimal(value / TB_BYTES, 1)} ТБ`;
  if (value >= GB_BYTES) return `${decimal(value / GB_BYTES, 1)} ГБ`;
  if (value >= MB_BYTES) return `${Math.round(value / MB_BYTES)} МБ`;
  if (value >= KB_BYTES) return `${Math.round(value / KB_BYTES)} КБ`;
  return `${Math.round(value)} Б`;
}

export function speedBytes(bytesPerSec: number) {
  const value = Math.max(0, bytesPerSec);
  if (value <= 0) return '0 КБ/с';
  if (value >= MB_BYTES) return `${decimal(value / MB_BYTES, 1)} МБ/с`;
  return `${Math.round(value / KB_BYTES)} КБ/с`;
}

export function rateMbText(bytesPerSec: number) {
  return String(Math.round((bytesPerSec / MB_BYTES) * 100) / 100).replace('.', ',');
}

export function rateLimitLabel(bytesPerSec: number) {
  if (bytesPerSec <= 0) return 'Без ограничений';
  return `${rateMbText(bytesPerSec)} МБ/с`;
}

export function truncateMiddle(text: string, max = 56) {
  if (text.length <= max) return text;
  const head = Math.ceil((max - 1) / 2);
  const tail = Math.floor((max - 1) / 2);
  return `${text.slice(0, head)}…${text.slice(text.length - tail)}`;
}

export function etaLabel(etaSeconds: number) {
  if (etaSeconds < 0) return '—';
  const min = Math.floor(etaSeconds / 60);
  const sec = Math.floor(etaSeconds % 60);
  if (min >= 60) {
    const h = Math.floor(min / 60);
    return `${h} ч ${min % 60} мин`;
  }
  return `${min} мин ${sec} сек`;
}
