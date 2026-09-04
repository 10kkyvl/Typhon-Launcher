import { msg } from '../i18n';
import type { SelfUpdateOutcome, SelfUpdateStatus } from './selfupdate';

function reasons(): [string, string][] {
  return [
    ['left the launcher binary unchanged', msg('state.selfupdateReasonBinaryUnchanged')],
    ['launcher did not exit', msg('state.selfupdateReasonLauncherDidNotExit')],
    ['downloaded hash differs', msg('state.selfupdateReasonCorruptDownload')],
    ['downloaded size differs', msg('state.selfupdateReasonCorruptDownload')],
    ['no verified update is ready', msg('state.selfupdateReasonInstallerNotFound')],
    ['run installer', msg('state.selfupdateReasonInstallerFailed')],
    ['artifact download stalled', msg('state.selfupdateReasonServerStalled')],
    ['Client.Timeout', msg('state.selfupdateReasonServerTimeout')],
    ['context deadline exceeded', msg('state.selfupdateReasonServerTimeout')],
    ['context canceled', msg('state.selfupdateReasonDownloadCancelled')],
    ['no such host', msg('state.selfupdateReasonCannotReachServer')],
    ['connection refused', msg('state.selfupdateReasonCannotReachServer')],
    ['dial tcp', msg('state.selfupdateReasonCannotReachServer')],
    ['tls:', msg('state.selfupdateReasonTlsFailed')],
    ['artifact endpoint returned an error status', msg('state.selfupdateReasonServerErrorStatus')],
    ['manifest endpoint returned an error status', msg('state.selfupdateReasonServerErrorStatus')],
    ['signature does not verify', msg('state.selfupdateReasonSignatureInvalid')],
    ['unknown key', msg('state.selfupdateReasonSignatureInvalid')],
    ['manifest is malformed', msg('state.selfupdateReasonManifestCorrupt')],
    ['manifest exceeds the size limit', msg('state.selfupdateReasonManifestCorrupt')],
    ['manifest version is not comparable', msg('state.selfupdateReasonManifestCorrupt')],
    ['manifest has no artifact for this platform', msg('state.selfupdateReasonPlatformUnsupported')],
    ['another update operation is in progress', msg('state.selfupdateReasonOperationInProgress')],
    ['check for an update before downloading', msg('state.selfupdateReasonCheckFirst')],
    ['not enough space', msg('state.selfupdateReasonNoSpace')],
    ['Access is denied', msg('state.selfupdateReasonAccessDenied')],
    ['permission denied', msg('state.selfupdateReasonAccessDenied')],
  ];
}

function codeReasons(): Record<string, string> {
  return {
    manifest: msg('state.selfupdateCodeCheckFailed'),
    artifact: msg('state.selfupdateCodeCheckFailed'),
    version: msg('state.selfupdateCodeCheckFailed'),
    download: msg('state.selfupdateCodeDownloadFailed'),
    apply: msg('state.selfupdateCodeApplyFailed'),
  };
}

function translate(raw: string): string {
  const known = reasons().find(([marker]) => raw.includes(marker));
  return known ? known[1] : '';
}

export function outcomeReason(outcome: SelfUpdateOutcome): string {
  const raw = outcome.error ?? '';
  return translate(raw) || raw;
}

export function updateReason(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err ?? '');
  return translate(raw) || raw;
}

export function statusReason(status: SelfUpdateStatus): string {
  const raw = status.error ?? '';
  if (!raw) return '';
  return translate(raw) || codeReasons()[status.errorCode ?? ''] || raw;
}
