export const COMPLETED_MIN = '1980-01-01';

function pad(value: number): string {
  return String(value).padStart(2, '0');
}

export function dayValue(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

export function dateInputValue(completedAt: string | null | undefined, now: Date): string {
  if (completedAt) {
    const parsed = new Date(completedAt);
    if (!Number.isNaN(parsed.getTime())) return dayValue(parsed);
  }
  return dayValue(now);
}

export function dateFromInput(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = new Date(year, month - 1, day, 12);
  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) return null;
  return date;
}

export function completionDateIssue(value: string, now: Date): boolean {
  const date = dateFromInput(value);
  if (!date) return true;
  return value < COMPLETED_MIN || value > dayValue(now);
}
