import type { DiscoveryResult } from '../services/recommendations';

const freshFor = 2 * 60 * 1000;
const maxEntries = 12;

// Keep the last usable shelf while revalidating it. Context keys include the
// filters, preferences, profile and library membership, so stale picks cannot
// cross a dismissal or a change of filters.
export function createDiscoveryCache(now = Date.now) {
  const entries = new Map<string, { result: DiscoveryResult; at: number }>();
  const pending = new Map<string, Promise<DiscoveryResult>>();

  return {
    get(key: string, load: () => Promise<DiscoveryResult>, force = false) {
      const previous = entries.get(key);
      if (previous) {
        entries.delete(key);
        entries.set(key, previous);
      }
      if (!force && !pending.has(key) && previous && !previous.result.fallback && now() - previous.at < freshFor) {
        return { cached: previous.result, refreshed: undefined };
      }
      let refreshed = pending.get(key);
      if (!refreshed || force) {
        const request = load().then((result) => {
          // Offline fallback must not replace a usable remote shelf or renew
          // its freshness. A later opening should retry the network.
          const retained = result.fallback && previous && !previous.result.fallback
            ? { ...previous.result, fallback: true } : result;
          if (pending.get(key) === request && retained === result) {
            entries.delete(key);
            entries.set(key, { result, at: now() });
            while (entries.size > maxEntries) entries.delete(entries.keys().next().value!);
          }
          return retained;
        }).finally(() => {
          if (pending.get(key) === request) pending.delete(key);
        });
        pending.set(key, request);
        refreshed = request;
      }
      return { cached: previous?.result, refreshed };
    },
  };
}

export const discoveryCache = createDiscoveryCache();
