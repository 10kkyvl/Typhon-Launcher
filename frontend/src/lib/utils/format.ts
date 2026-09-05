import { locale, translator, type Locale, type Translate } from '../i18n';

const KB_BYTES = 1024;
const MB_BYTES = 1024 ** 2;
const GB_BYTES = 1024 ** 3;
const TB_BYTES = 1024 ** 4;

const DASH = '—';

export function bytesToGb(bytes: number) {
  return bytes / GB_BYTES;
}

export function truncateMiddle(text: string, max = 56) {
  if (text.length <= max) return text;
  const head = Math.ceil((max - 1) / 2);
  const tail = Math.floor((max - 1) / 2);
  return `${text.slice(0, head)}…${text.slice(text.length - tail)}`;
}

export type Format = ReturnType<typeof makeFormat>;

export function makeFormat(loc: Locale, t: Translate) {
  const num = (value: number, min = 0, max = min) =>
    new Intl.NumberFormat(loc, { minimumFractionDigits: min, maximumFractionDigits: max }).format(value);

  const date = new Intl.DateTimeFormat(loc);
  const longDateFormat = new Intl.DateTimeFormat(loc, { day: 'numeric', month: 'long', year: 'numeric' });
  const shortDateFormat = new Intl.DateTimeFormat(loc, { day: 'numeric', month: 'short' });
  const dateTimeFormat = new Intl.DateTimeFormat(loc, { dateStyle: 'short', timeStyle: 'medium' });
  const clockFormat = new Intl.DateTimeFormat(loc, { hour: '2-digit', minute: '2-digit' });
  const weekdayFormat = new Intl.DateTimeFormat(loc, { weekday: 'short' });

  const clock = (seconds: number) => {
    const min = Math.floor(seconds / 60);
    const sec = Math.floor(seconds % 60);
    if (min >= 60) {
      const h = Math.floor(min / 60);
      return `${h} ${t('units.hour')} ${min % 60} ${t('units.minute')}`;
    }
    return `${min} ${t('units.minute')} ${sec} ${t('units.second')}`;
  };

  const editable = new Intl.NumberFormat(loc, { maximumFractionDigits: 2, useGrouping: false });

  const rateMbText = (bytesPerSec: number) => editable.format(Math.round((bytesPerSec / MB_BYTES) * 100) / 100);

  return {
    gb: (value: number, digits = 1) => `${num(value, digits)} ${t('units.gb')}`,

    speed: (mbs: number) => `${num(mbs, 1)} ${t('units.mbs')}`,

    eta: (doneGb: number, totalGb: number, speedMbs: number) =>
      speedMbs <= 0 ? DASH : clock(((totalGb - doneGb) * 1024) / speedMbs),

    etaLabel: (etaSeconds: number) => (etaSeconds < 0 ? DASH : clock(etaSeconds)),

    formatCount: (n: number) => num(n),

    playtime: (seconds: number) => {
      if (seconds < 60) return seconds > 0 ? t('format.lessThanMinute') : DASH;
      const hours = Math.floor(seconds / 3600);
      const minutes = Math.floor((seconds % 3600) / 60);
      if (hours === 0) return `${num(minutes)} ${t('units.minute')}`;
      if (hours < 10 && minutes > 0) return `${num(hours)} ${t('units.hour')} ${num(minutes)} ${t('units.minute')}`;
      return `${num(hours)} ${t('units.hour')}`;
    },

    relativeDate: (iso: string | null) => {
      if (!iso) return DASH;
      const value = new Date(iso);
      if (Number.isNaN(value.getTime())) return DASH;
      const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
      const days = Math.round((startOfDay(new Date()) - startOfDay(value)) / 86400000);
      if (days <= 0) return t('format.today');
      if (days === 1) return t('format.yesterday');
      if (days < 7) return t('format.daysAgo', { count: days });
      if (days < 30) return t('format.weeksAgo', { count: Math.round(days / 7) });
      return date.format(value);
    },

    bytesLabel: (bytes: number) => {
      const gbValue = bytes / GB_BYTES;
      if (gbValue >= 1000) return `${num(Math.round((gbValue / 1024) * 10) / 10, 0, 1)} ${t('units.tb')}`;
      return `${num(Math.round(gbValue))} ${t('units.gb')}`;
    },

    bytesSize: (bytes: number) => {
      const value = Math.max(0, bytes);
      if (value >= TB_BYTES) return `${num(value / TB_BYTES, 1)} ${t('units.tb')}`;
      if (value >= GB_BYTES) return `${num(value / GB_BYTES, 1)} ${t('units.gb')}`;
      if (value >= MB_BYTES) return `${num(Math.round(value / MB_BYTES))} ${t('units.mb')}`;
      if (value >= KB_BYTES) return `${num(Math.round(value / KB_BYTES))} ${t('units.kb')}`;
      return `${num(Math.round(value))} ${t('units.b')}`;
    },

    speedBytes: (bytesPerSec: number) => {
      const value = Math.max(0, bytesPerSec);
      if (value <= 0) return `${num(0)} ${t('units.kbs')}`;
      if (value >= MB_BYTES) return `${num(value / MB_BYTES, 1)} ${t('units.mbs')}`;
      return `${num(Math.round(value / KB_BYTES))} ${t('units.kbs')}`;
    },

    longDate: (value: Date) => longDateFormat.format(value),

    shortDate: (value: Date) => shortDateFormat.format(value),

    numericDate: (value: Date) => date.format(value),

    dateTime: (value: Date) => dateTimeFormat.format(value),

    clockTime: (value: Date) => clockFormat.format(value),

    weekday: (value: Date) => weekdayFormat.format(value),

    rateMbText,

    rateLimitLabel: (bytesPerSec: number) =>
      bytesPerSec <= 0 ? t('format.noLimit') : `${rateMbText(bytesPerSec)} ${t('units.mbs')}`,
  };
}

let current = makeFormat('ru', translator('ru'));

locale.subscribe((loc) => {
  current = makeFormat(loc, translator(loc));
});

export const gb: Format['gb'] = (value, digits) => current.gb(value, digits);
export const speed: Format['speed'] = (mbs) => current.speed(mbs);
export const eta: Format['eta'] = (done, total, rate) => current.eta(done, total, rate);
export const etaLabel: Format['etaLabel'] = (seconds) => current.etaLabel(seconds);
export const formatCount: Format['formatCount'] = (n) => current.formatCount(n);
export const playtime: Format['playtime'] = (seconds) => current.playtime(seconds);
export const relativeDate: Format['relativeDate'] = (iso) => current.relativeDate(iso);
export const bytesLabel: Format['bytesLabel'] = (bytes) => current.bytesLabel(bytes);
export const bytesSize: Format['bytesSize'] = (bytes) => current.bytesSize(bytes);
export const speedBytes: Format['speedBytes'] = (bytes) => current.speedBytes(bytes);
export const rateMbText: Format['rateMbText'] = (bytes) => current.rateMbText(bytes);
export const longDate: Format['longDate'] = (value) => current.longDate(value);
export const shortDate: Format['shortDate'] = (value) => current.shortDate(value);
export const numericDate: Format['numericDate'] = (value) => current.numericDate(value);
export const dateTime: Format['dateTime'] = (value) => current.dateTime(value);
export const clockTime: Format['clockTime'] = (value) => current.clockTime(value);
export const weekday: Format['weekday'] = (value) => current.weekday(value);
export const rateLimitLabel: Format['rateLimitLabel'] = (bytes) => current.rateLimitLabel(bytes);
