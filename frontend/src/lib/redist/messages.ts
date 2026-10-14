import { errorCode, hasMessage, msg, type MessageKey } from '../i18n';
import type { PackageKind, RedistResult } from '../services/redist';

const KIND_KEYS: Record<PackageKind, MessageKey> = {
  vcredist: 'redist.kindVcredist',
  directx: 'redist.kindDirectx',
  dotnet: 'redist.kindDotnet',
  other: 'redist.kindOther',
};

export function redistCodeText(code: string, fallback: string = msg('redist.fallback')): string {
  return code.startsWith('redist.') && hasMessage(code) ? msg(code) : fallback;
}

export function redistErrorText(err: unknown, fallback: string = msg('redist.fallback')): string {
  return redistCodeText(errorCode(err), fallback);
}

export function isUnsupported(err: unknown): boolean {
  return errorCode(err) === 'redist.unsupported';
}

export function kindLabel(kind: PackageKind): string {
  const key = KIND_KEYS[kind];
  return key ? msg(key) : kind;
}

export function resultText(result: RedistResult, name: string): string {
  if (result.alreadyInstalled) return msg('redist.resultAlready', { name });
  if (result.rebootRequired) return msg('redist.resultReboot', { name });
  return msg('redist.resultOk', { name });
}

export function crashReasonText(code: string): string {
  const key = `crash.reason.${code}`;
  return code && hasMessage(key) ? msg(key) : '';
}
