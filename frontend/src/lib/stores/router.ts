import { derived, get, writable } from 'svelte/store';

export type RouteName =
  | 'library'
  | 'catalog'
  | 'game'
  | 'downloads'
  | 'sources'
  | 'installed'
  | 'history'
  | 'lan'
  | 'friends'
  | 'activity'
  | 'user'
  | 'profile'
  | 'settings';

export interface Route {
  name: RouteName;
  params: Record<string, string>;
  key: number;
}

let nextKey = 0;

function entry(name: RouteName, params: Record<string, string>): Route {
  return { name, params, key: ++nextKey };
}

const memory = new Map<number, Map<string, unknown>>();

const history = writable<{ stack: Route[]; index: number }>({
  stack: [entry('library', {})],
  index: 0,
});

export const route = derived(history, ($h) => $h.stack[$h.index]);
export const canGoBack = derived(history, ($h) => $h.index > 0);
export const canGoForward = derived(history, ($h) => $h.index < $h.stack.length - 1);

function prune(stack: Route[]) {
  const alive = new Set(stack.map((item) => item.key));
  for (const key of [...memory.keys()]) {
    if (!alive.has(key)) memory.delete(key);
  }
}

export function navigate(name: RouteName, params: Record<string, string> = {}) {
  const h = get(history);
  const current = h.stack[h.index];
  if (current.name === name && JSON.stringify(current.params) === JSON.stringify(params)) return;
  const stack = h.stack.slice(0, h.index + 1);
  stack.push(entry(name, params));
  prune(stack);
  history.set({ stack, index: stack.length - 1 });
}

export function goBack() {
  history.update((h) => (h.index > 0 ? { ...h, index: h.index - 1 } : h));
}

export function goForward() {
  history.update((h) => (h.index < h.stack.length - 1 ? { ...h, index: h.index + 1 } : h));
}

export function resetHistory() {
  memory.clear();
  history.set({ stack: [entry('library', {})], index: 0 });
}

export function currentRouteKey(): number {
  return get(route).key;
}

export function stashRoute(key: number, slot: string, value: unknown) {
  let slots = memory.get(key);
  if (!slots) {
    slots = new Map<string, unknown>();
    memory.set(key, slots);
  }
  slots.set(slot, value);
}

export function recallRoute<T>(key: number, slot: string): T | undefined {
  return memory.get(key)?.get(slot) as T | undefined;
}
