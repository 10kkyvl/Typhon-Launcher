import { errorCode, msg, type MessageKey } from '../i18n';

const REASONS: Record<string, MessageKey> = {
  'updates.not_tracked': 'errUpdates.updatesNotTracked',
  'updates.no_install_dir': 'errUpdates.updatesNoInstallDir',
  'updates.no_plan': 'errUpdates.updatesNoPlan',
  'updates.game_running': 'errUpdates.updatesGameRunning',
  'updates.busy': 'errUpdates.updatesBusy',
  'updates.no_rollback': 'errUpdates.updatesNoRollback',
  'updates.no_identity': 'errUpdates.updatesNoIdentity',
  'updates.no_downloads': 'errUpdates.updatesNoDownloads',
  'updates.no_installer': 'errUpdates.updatesNoInstaller',
  'updates.no_library': 'errUpdates.updatesNoLibrary',
  'updates.update_failed': 'errUpdates.updatesUpdateFailed',
  'updates.download_failed': 'errUpdates.updatesDownloadFailed',
  'updates.install_failed': 'errUpdates.updatesInstallFailed',
  'updates.staging_empty': 'errUpdates.updatesStagingEmpty',
  'updates.no_launch_target': 'errUpdates.updatesNoLaunchTarget',
  'updates.swap_failed': 'errUpdates.updatesSwapFailed',
  'updates.carry_over_failed': 'errUpdates.updatesCarryOverFailed',
  'updates.prefetch_unavailable': 'errUpdates.updatesPrefetchUnavailable',
  'updates.no_free_space_for_backup': 'errUpdates.updatesNoFreeSpaceForBackup',
  'updates.download_stalled': 'errUpdates.updatesDownloadStalled',
  'updates.no_target': 'errUpdates.updatesNoTarget',
  'updates.repair_unavailable': 'errUpdates.updatesRepairUnavailable',
};

export function updateErrorText(raw: unknown, fallback: string = msg('errUpdates.fallback')): string {
  const key = REASONS[errorCode(raw)];
  return key ? msg(key) : fallback;
}
