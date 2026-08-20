import { writable } from 'svelte/store';
import { initialSources } from '../mock/sources';
import type { Source } from '../mock/types';

export const sources = writable<Source[]>(structuredClone(initialSources));

export function toggleSource(id: string) {
  sources.update((list) =>
    list.map((s) => (s.id === id ? { ...s, status: s.status === 'disabled' ? 'active' : 'disabled' } : s)),
  );
}

export function removeSource(id: string) {
  sources.update((list) => list.filter((s) => s.id !== id));
}

export function addSource(name: string, path: string) {
  sources.update((list) => [
    ...list,
    {
      id: `src-${Date.now()}`,
      name,
      path,
      status: 'active',
      lastUpdate: 'Только что',
      items: 0,
      version: '1.0.0',
      kind: 'web',
    },
  ]);
}
