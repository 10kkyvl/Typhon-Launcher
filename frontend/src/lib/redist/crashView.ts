import type { MessageKey } from '../i18n';
import type { CrashEvent, CrashHint, SessionEvent } from '../services/library';

export const CRASH_DETAIL_WINDOW_MS = 15 * 1000;

export interface CrashDetail {
  faultModule: string;
  exceptionCode: string;
  hint: CrashHint | '';
  missingFile: string;
  dumpFound: boolean;
  journalUnknown: boolean;
}

export interface CrashRecord {
  gameId: string;
  exitCode: number;
  exitKnown: boolean;
  reasonCode: string;
  sessionSeconds: number;
  at: number;
  count: number;
  late?: boolean;
  detail?: CrashDetail;
}

export type CrashAction = 'libs' | 'verify' | 'compat' | 'report';

export interface CrashLine {
  key: MessageKey;
  params?: Record<string, string | number>;
}

export interface CrashView {
  title: MessageKey;
  reason: CrashLine | null;
  tips: MessageKey[];
  tech: CrashLine[];
  actions: CrashAction[];
}

const HINTS: readonly string[] = ['vcredist', 'directx', 'compat', 'antivirus', 'runtimes', 'unknown'];

export function toDetail(event: CrashEvent): CrashDetail {
  return {
    faultModule: event.faultModule ?? '',
    exceptionCode: event.exceptionCode ?? '',
    hint: HINTS.includes(event.hint) ? (event.hint as CrashHint) : '',
    missingFile: event.missingFile ?? '',
    dumpFound: !!event.dumpFound,
    journalUnknown: !!event.journalUnknown,
  };
}

export function recordFromStopped(event: SessionEvent, count: number, now: number): CrashRecord {
  return {
    gameId: event.gameId,
    exitCode: event.exitCode,
    exitKnown: event.exitKnown,
    reasonCode: event.reasonCode,
    sessionSeconds: event.sessionSeconds,
    at: now,
    count,
  };
}

export function recordFromCrash(event: CrashEvent, count: number, now: number): CrashRecord {
  return {
    gameId: event.gameId,
    exitCode: event.exitCode,
    exitKnown: event.exitKnown,
    reasonCode: event.reasonCode,
    sessionSeconds: event.sessionSeconds,
    at: now,
    count,
    late: true,
    detail: toDetail(event),
  };
}

export function mergeCrash(record: CrashRecord, event: CrashEvent): CrashRecord {
  return {
    ...record,
    exitCode: event.exitKnown ? event.exitCode : record.exitCode,
    exitKnown: record.exitKnown || event.exitKnown,
    reasonCode: event.reasonCode || record.reasonCode,
    detail: toDetail(event),
  };
}

function reasonLine(detail: CrashDetail): CrashLine | null {
  const module = detail.faultModule;
  switch (detail.hint) {
    case 'vcredist':
      return module ? { key: 'redist.hintVcredist', params: { module } } : { key: 'redist.hintVcredistBare' };
    case 'directx':
      return module ? { key: 'redist.hintDirectx', params: { module } } : { key: 'redist.hintDirectxBare' };
    case 'compat':
      return module ? { key: 'redist.hintCompat', params: { module } } : { key: 'redist.hintCompatBare' };
    case 'antivirus':
      return detail.missingFile
        ? { key: 'redist.hintAntivirus', params: { file: detail.missingFile } }
        : { key: 'redist.hintAntivirusBare' };
    case 'runtimes':
      return { key: 'redist.hintRuntimes' };
    case 'unknown':
      return { key: 'redist.hintUnknown' };
    default:
      return null;
  }
}

function actionsFor(hint: CrashDetail['hint']): CrashAction[] {
  switch (hint) {
    case 'compat':
      return ['compat', 'verify'];
    case 'antivirus':
      return ['verify'];
    case 'unknown':
      return ['report', 'verify'];
    default:
      return ['libs', 'verify'];
  }
}

function exitLine(record: CrashRecord): CrashLine[] {
  if (!record.exitKnown) return [];
  return [{ key: 'redist.crashCode', params: { code: `0x${(record.exitCode >>> 0).toString(16).toUpperCase()}` } }];
}

export function crashView(record: CrashRecord): CrashView {
  const detail = record.detail;
  const title: MessageKey = record.late ? 'redist.crashTitleLate' : 'redist.crashTitle';
  if (!detail) {
    return { title, reason: null, tips: [], tech: exitLine(record), actions: actionsFor('') };
  }
  const tech: CrashLine[] = [];
  if (detail.faultModule) tech.push({ key: 'redist.crashModule', params: { module: detail.faultModule } });
  if (detail.exceptionCode) tech.push({ key: 'redist.crashException', params: { code: detail.exceptionCode } });
  tech.push(...exitLine(record));
  const tips: MessageKey[] = [];
  if (detail.hint === 'antivirus') tips.push('redist.crashAntivirusTip');
  if (detail.dumpFound) tips.push('redist.crashDump');
  if (detail.journalUnknown) tips.push('redist.crashJournalUnknown');
  return { title, reason: reasonLine(detail), tips, tech, actions: actionsFor(detail.hint) };
}
