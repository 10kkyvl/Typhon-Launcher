import { msg } from '../../lib/i18n';
import { clockTime, longDate, shortDate } from '../../lib/utils/format';

/**
 * When a message was written: short words for the screen (the clock time today, the day and the
 * time before), and the exact time with the year for assistive tech and the hint, as the History
 * screen says it. Null for a time that cannot be read.
 */
export function messageWhen(iso: string, now: Date = new Date()): { words: string; exact: string } | null {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return null;
  const time = clockTime(date);
  const today = date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate();
  return {
    words: today ? time : msg('tickets.messageTime', { date: shortDate(date), time }),
    exact: msg('tickets.messageTime', { date: longDate(date), time }),
  };
}
