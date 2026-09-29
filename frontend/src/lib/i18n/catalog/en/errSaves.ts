import type { Message } from '../../types';
import type { ErrSavesKey } from '../ru/errSaves';

export const errSaves: Record<ErrSavesKey, Message> = {
  'savebackup.fallback': 'Could not complete the backup action',
  'savebackup.not_started': 'The backup service is not running yet',
  'savebackup.invalid_id': 'Invalid game or backup identifier',
  'savebackup.invalid_kind': 'Unknown backup kind',
  'savebackup.saves_not_found': 'Save folder not found. Specify it manually',
  'savebackup.saves_ambiguous': 'Several save folders were found. Choose the right one',
  'savebackup.saves_not_a_directory': 'The saves path is not a folder',
  'savebackup.saves_path_unavailable': 'The save folder is unavailable',
  'savebackup.no_source': 'No folder to copy was given',
  'savebackup.game_running': 'Close the game before restoring',
  'savebackup.snapshot_not_found': 'Backup not found',
  'savebackup.snapshot_broken': 'The backup is damaged and cannot be restored',
  'savebackup.verify_failed': 'The copy did not match the original. Try again',
  'savebackup.unchanged': 'Saves have not changed since the last backup',
  'savebackup.restore_leftovers':
    'Leftovers of an interrupted restore remain next to the save folder. Check the folder and try again',
  'savebackup.recovery_failed': 'Could not finish an interrupted operation on the saves',
  'savebackup.no_free_space': 'Not enough disk space for the backup',
  'savebackup.rotation_failed': 'The backup was created, but old backups could not be removed',
  'savebackup.cleanup_failed': 'temporary files next to the save folder could not be removed',
};
