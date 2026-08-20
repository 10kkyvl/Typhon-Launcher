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
