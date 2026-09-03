export function markError(err: unknown, fallback: string): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  if (raw.includes('favorites limit reached')) return 'Не больше 6 любимых игр';
  return fallback;
}
