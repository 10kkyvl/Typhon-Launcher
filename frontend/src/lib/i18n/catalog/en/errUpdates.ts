import type { Message } from '../../types';
import type { ErrUpdatesKey } from '../ru/errUpdates';

export const errUpdates: Record<ErrUpdatesKey, Message> = {
  'errUpdates.updatesNotTracked': 'No update data for this game',
  'errUpdates.updatesNoInstallDir': 'The game has no install directory set',
  'errUpdates.updatesNoPlan': 'Prepare an update plan first',
  'errUpdates.updatesGameRunning': 'The game is running — close it before updating',
  'errUpdates.updatesBusy': 'An operation is already in progress',
  'errUpdates.updatesNoRollback': 'The previous version is not available',
  'errUpdates.updatesNoIdentity': 'Verification is not available for this installation',
  'errUpdates.updatesNoDownloads': 'The download manager is unavailable',
  'errUpdates.updatesNoInstaller': 'The installer is unavailable',
  'errUpdates.updatesNoLibrary': 'The library is unavailable',
  'errUpdates.updatesUpdateFailed': 'Failed to apply the update',
  'errUpdates.updatesDownloadFailed': 'Failed to download the update data',
  'errUpdates.updatesInstallFailed': 'Failed to install the update',
  'errUpdates.updatesStagingEmpty': 'The temporary installation is empty',
  'errUpdates.updatesNoLaunchTarget': 'No executable found after the update',
  'errUpdates.updatesSwapFailed': 'Failed to replace the installed version',
  'errUpdates.updatesCarryOverFailed': 'Failed to carry over user files from the previous version',
  'errUpdates.updatesPrefetchUnavailable': 'Prefetching is not available for this strategy',
  'errUpdates.updatesNoFreeSpaceForBackup': 'Not enough disk space for a backup before updating',
  'errUpdates.updatesDownloadStalled': 'The download stalled: no network or sources, try updating again later',
  'errUpdates.updatesNoTarget': 'No release available for the update',
  'errUpdates.updatesRepairUnavailable': 'Repair is not available for this installation',
  'errUpdates.selfupdateReadOnly':
    "Couldn't save update data because of an earlier read error. Restart Typhon",
  'errUpdates.selfupdateManifestOutdated':
    'This version of Typhon is no longer supported by the update server. Install the latest version manually',
};
