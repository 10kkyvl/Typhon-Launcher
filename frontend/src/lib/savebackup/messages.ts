import { errorCode, hasMessage, msg, type MessageKey } from '../i18n';
import type { Snapshot, SnapshotKind } from '../services/savebackup';
import { dateTime } from '../utils/format';

const KIND_KEYS: Record<SnapshotKind, MessageKey> = {
  manual: 'saves.kindManual',
  session: 'saves.kindSession',
  update: 'saves.kindUpdate',
  'pre-restore': 'saves.kindPreRestore',
};

export function saveBackupCodeText(code: string, fallback: string): string {
  const known = code.startsWith('savebackup.') || code.startsWith('library.');
  return known && hasMessage(code) ? msg(code) : fallback;
}

export function saveBackupErrorText(err: unknown, fallback: string = msg('savebackup.fallback')): string {
  return saveBackupCodeText(errorCode(err), fallback);
}

export function snapshotKindLabel(kind: SnapshotKind): string {
  const key = KIND_KEYS[kind];
  return key ? msg(key) : kind;
}

export function snapshotWhen(snapshot: Pick<Snapshot, 'id' | 'createdAt'>): string {
  const value = new Date(snapshot.createdAt);
  return Number.isNaN(value.getTime()) ? snapshot.id : dateTime(value);
}
