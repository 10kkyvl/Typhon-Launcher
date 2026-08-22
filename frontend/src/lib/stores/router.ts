import { derived, get, writable } from 'svelte/store';

export type RouteName =
  | 'library'
  | 'game'
  | 'downloads'
  | 'sources'
  | 'installed'
  | 'collections'
  | 'achievements'
  | 'profile'
  | 'settings';

export interface Route {
  name: RouteName;
  params: Record<string, string>;
}

const history = writable<{ stack: Route[]; index: number }>({
  stack: [{ name: 'library', params: {} }],
  index: 0,
});

export const route = derived(history, ($h) => $h.stack[$h.index]);
export const canGoBack = derived(history, ($h) => $h.index > 0);
export const canGoForward = derived(history, ($h) => $h.index < $h.stack.length - 1);

export function navigate(name: RouteName, params: Record<string, string> = {}) {
  const h = get(history);
  const current = h.stack[h.index];
  if (current.name === name && JSON.stringify(current.params) === JSON.stringify(params)) return;
  const stack = h.stack.slice(0, h.index + 1);
  stack.push({ name, params });
  history.set({ stack, index: stack.length - 1 });
}

export function goBack() {
  history.update((h) => (h.index > 0 ? { ...h, index: h.index - 1 } : h));
}

export function goForward() {
  history.update((h) => (h.index < h.stack.length - 1 ? { ...h, index: h.index + 1 } : h));
}

export function resetHistory() {
  history.set({ stack: [{ name: 'library', params: {} }], index: 0 });
}
