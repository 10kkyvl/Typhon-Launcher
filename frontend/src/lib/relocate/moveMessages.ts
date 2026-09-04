import { errorCode, msg } from '../i18n';
import type { MessageKey } from '../i18n';

export const REASONS: Record<string, MessageKey> = {
  'relocate.game_running': 'transfers.moveErrGameRunning',
  'relocate.updating': 'transfers.moveErrUpdating',
  'relocate.installing': 'transfers.moveErrInstalling',
  'relocate.downloading': 'transfers.moveErrDownloading',
  'relocate.already_running': 'transfers.moveErrAlreadyRunning',
  'relocate.target_not_empty': 'transfers.moveErrTargetNotEmpty',
  'relocate.target_inside_source': 'transfers.moveErrTargetInsideSource',
  'relocate.source_inside_target': 'transfers.moveErrSourceInsideTarget',
  'relocate.target_is_drive_root': 'transfers.moveErrTargetIsDriveRoot',
  'relocate.invalid_path': 'transfers.moveErrInvalidPath',
  'relocate.no_target': 'transfers.moveErrNoTarget',
  'relocate.no_source': 'transfers.moveErrNoSource',
  'relocate.free_space_unknown': 'transfers.moveErrFreeSpaceUnknown',
  'relocate.not_enough_space': 'transfers.moveErrNotEnoughSpace',
  'relocate.verify_failed': 'transfers.moveErrVerifyFailed',
  'relocate.no_install_dir': 'transfers.moveErrNoInstallDir',
  'relocate.job_not_found': 'transfers.moveErrJobNotFound',
  'relocate.game_not_found': 'transfers.moveErrGameNotFound',
  'relocate.ambiguous_recovery': 'transfers.moveErrAmbiguousRecovery',
  'relocate.dialog_unavailable': 'transfers.moveErrDialogUnavailable',
  'relocate.shutting_down': 'transfers.moveErrShuttingDown',
  'relocate.not_ready': 'transfers.moveErrNotReady',
};

export function moveErrorText(err: unknown, fallback: string = msg('transfers.moveErrFallback')): string {
  const key = REASONS[errorCode(err)];
  return key ? msg(key) : fallback;
}
