import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';
import type { SelfUpdateOutcome, SelfUpdateStatus } from './selfupdate';

const REASONS: Record<string, MessageKey> = {
  'selfupdate.not_replaced': 'state.selfupdateReasonBinaryUnchanged',
  'selfupdate.parent_still_running': 'state.selfupdateReasonLauncherDidNotExit',
  'selfupdate.hash_mismatch': 'state.selfupdateReasonCorruptDownload',
  'selfupdate.size_mismatch': 'state.selfupdateReasonCorruptDownload',
  'selfupdate.not_ready': 'state.selfupdateReasonInstallerNotFound',
  'selfupdate.stalled': 'state.selfupdateReasonServerStalled',
  'selfupdate.artifact_status': 'state.selfupdateReasonServerErrorStatus',
  'selfupdate.manifest_status': 'state.selfupdateReasonServerErrorStatus',
  'selfupdate.bad_signature': 'state.selfupdateReasonSignatureInvalid',
  'selfupdate.unknown_key': 'state.selfupdateReasonSignatureInvalid',
  'selfupdate.bad_public_key': 'state.selfupdateReasonSignatureInvalid',
  'selfupdate.invalid_manifest': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.manifest_too_large': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_version': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_version_path': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.empty_config_dir': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.unsupported_artifact_kind': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_artifact': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_artifact_name': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_artifact_url': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_artifact_size': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_hash': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_release_note': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_change_kind': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.empty_note_text': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.invalid_note_text': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.too_many_release_notes': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.unordered_release_notes': 'state.selfupdateReasonManifestCorrupt',
  'selfupdate.no_artifact': 'state.selfupdateReasonPlatformUnsupported',
  'selfupdate.apply_unsupported': 'state.selfupdateReasonPlatformUnsupported',
  'selfupdate.busy': 'state.selfupdateReasonOperationInProgress',
  'selfupdate.check_first': 'state.selfupdateReasonCheckFirst',
  'selfupdate.read_only': 'errUpdates.selfupdateReadOnly',
  'selfupdate.manifest_outdated': 'errUpdates.selfupdateManifestOutdated',
  'selfupdate.installer_failed': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.install_dir_empty': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.install_dir_not_absolute': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.install_dir_not_clean': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.install_dir_unsafe': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.install_dir_not_dir': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.installer_path_not_absolute': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.installer_path_not_clean': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.installer_outside_cache': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.installer_not_regular_file': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.installer_path_unsafe': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.launcher_path_not_absolute': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.launcher_path_not_clean': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.launcher_path_outside_install': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.launcher_path_not_regular': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.launcher_digest_mismatch': 'state.selfupdateReasonInstallerFailed',
  'selfupdate.relaunch_path_empty': 'state.selfupdateReasonInstallerFailed',
};

// net/http, crypto/tls and the OS surface these directly, with no sentinel of
// ours to attach a code to; classifying them in Go would mean intercepting
// every dial/read/write call in the package, so their own stable english
// wording is matched here instead, once, as a last resort.
const RESIDUAL: [string, MessageKey][] = [
  ['Client.Timeout', 'state.selfupdateReasonServerTimeout'],
  ['context deadline exceeded', 'state.selfupdateReasonServerTimeout'],
  ['context canceled', 'state.selfupdateReasonDownloadCancelled'],
  ['no such host', 'state.selfupdateReasonCannotReachServer'],
  ['connection refused', 'state.selfupdateReasonCannotReachServer'],
  ['dial tcp', 'state.selfupdateReasonCannotReachServer'],
  ['tls:', 'state.selfupdateReasonTlsFailed'],
  ['not enough space', 'state.selfupdateReasonNoSpace'],
  ['Access is denied', 'state.selfupdateReasonAccessDenied'],
  ['permission denied', 'state.selfupdateReasonAccessDenied'],
];

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
  const key = REASONS[errorCode(raw)];
  if (key) return msg(key);
  const known = RESIDUAL.find(([marker]) => raw.includes(marker));
  return known ? msg(known[1]) : '';
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
