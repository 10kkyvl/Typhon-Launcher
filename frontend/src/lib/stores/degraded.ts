import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { msg } from '../i18n';
import { toast } from './toasts';

export interface DegradedStatus {
  degraded: boolean;
  message: string;
}

// Каждая подсистема сообщает о неудачной записи своего состояния отдельным
// событием: бэкенд откатывает память и остаётся жить, а пользователь узнаёт
// об этом только отсюда.
const EVENTS = [
  'download:degraded',
  'update:degraded',
  'source:degraded',
  'playlog:degraded',
  'move:degraded',
] as const;

export type DegradedEvent = (typeof EVENTS)[number];

export const degraded = writable<Record<string, string>>({});

// Событие приходит на каждую неудачную запись, а не только на смену
// состояния: при заполненном диске это поток. Тост показывается один раз на
// подсистему, пока она не восстановится.
export function applyDegraded(name: DegradedEvent, status: DegradedStatus): boolean {
  let notify = false;
  degraded.update((current) => {
    const wasDegraded = name in current;
    if (!status.degraded) {
      if (!wasDegraded) return current;
      const next = { ...current };
      delete next[name];
      return next;
    }
    notify = !wasDegraded;
    return { ...current, [name]: status.message };
  });
  return notify;
}

export function initDegradedNotices() {
  if (!inWails) return;
  for (const name of EVENTS) {
    Events.On(name, (event) => {
      const status = event.data as DegradedStatus;
      if (!status) return;
      if (applyDegraded(name, status)) {
        toast(msg('state.stateSaveFailed', { message: status.message }), 'danger');
      }
    });
  }
}
