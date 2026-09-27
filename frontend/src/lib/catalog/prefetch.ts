export function createPagePrefetch<T>() {
  let pending: { key: string; value: Promise<T | null> } | undefined;
  return {
    warm(key: string, load: () => Promise<T>) {
      if (pending?.key === key) return;
      pending = { key, value: load().catch(() => null) };
    },
    async take(key: string, load: () => Promise<T>): Promise<T> {
      const cached = pending?.key === key ? pending.value : undefined;
      pending = undefined;
      const result = cached ? await cached : null;
      return result ?? load();
    },
    clear() { pending = undefined; },
  };
}
