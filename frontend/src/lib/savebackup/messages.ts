import { errorCode, hasMessage, msg, type MessageKey } from '../i18n';
import type { BackupEvent, Snapshot, SnapshotKind } from '../services/savebackup';
import { dateTime, playtime } from '../utils/format';

const KIND_KEYS: Record<SnapshotKind, MessageKey> = {
  manual: 'saves.kindManual',
  session: 'saves.kindSession',
  update: 'saves.kindUpdate',
  'pre-restore': 'saves.kindPreRestore',
  cloud: 'saves.kindCloud',
  launch: 'saves.kindLaunch',
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

export function snapshotPlayedAfter(snapshot: Pick<Snapshot, 'playedAfterKnown' | 'playedAfterSeconds'>): string {
  if (!snapshot.playedAfterKnown) return '';
  const seconds = snapshot.playedAfterSeconds ?? 0;
  if (seconds <= 0) return msg('saves.playedAfterNone');
  return msg('saves.playedAfter', { time: playtime(seconds) });
}

export interface BackupNotice {
  text: string;
  kind: 'danger' | 'info';
}

export function backupNotice(event: BackupEvent, title: string): BackupNotice | null {
  if (event.kind === 'manual') return null;
  const reason = () => saveBackupCodeText(event.code, msg('savebackup.fallback'));
  if (event.status === 'failed') {
    const key = event.kind === 'launch' ? 'saves.launchFailed' : 'saves.autoFailed';
    return { text: msg(key, { title, reason: reason() }), kind: 'danger' };
  }
  if (event.status === 'created' && event.error) {
    return { text: msg('saves.autoRotationFailed', { title, reason: reason() }), kind: 'info' };
  }
  return null;
}
