import { writable } from 'svelte/store';

export interface Toast {
  id: number;
  message: string;
  kind: 'info' | 'success' | 'danger';
}

export const toasts = writable<Toast[]>([]);

let nextId = 1;

export function toast(message: string, kind: Toast['kind'] = 'info') {
  const id = nextId++;
  toasts.update((list) => [...list, { id, message, kind }]);
  setTimeout(() => {
    toasts.update((list) => list.filter((t) => t.id !== id));
  }, 3500);
}
