import { writable } from 'svelte/store';

export interface ToastAction {
  label: string;
  run: () => void;
}

export interface Toast {
  id: number;
  message: string;
  kind: 'info' | 'success' | 'danger';
  action?: ToastAction;
}

export const toasts = writable<Toast[]>([]);

const PLAIN_MS = 3500;
const ACTION_MS = 9000;

let nextId = 1;

export function dismissToast(id: number) {
  toasts.update((list) => list.filter((t) => t.id !== id));
}

export function toast(message: string, kind: Toast['kind'] = 'info', action?: ToastAction) {
  const id = nextId++;
  toasts.update((list) => [...list, action ? { id, message, kind, action } : { id, message, kind }]);
  setTimeout(() => dismissToast(id), action ? ACTION_MS : PLAIN_MS);
  return id;
}
