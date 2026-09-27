import { ru, type MessageKey } from './catalog/ru';

const CODE = /typhon:([a-z0-9_]+(?:\.[a-z0-9_]+)*)/;

export function errorCode(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  return CODE.exec(raw)?.[1] ?? '';
}

export function hasMessage(key: string): key is MessageKey {
  return Object.prototype.hasOwnProperty.call(ru, key);
}
