import type { Message } from '../../types';
import type { InstalledKey } from '../ru/installed';

export const installed: Record<InstalledKey, Message> = {
  'installed.youHave': { one: 'You have {count} game installed', other: 'You have {count} games installed' },
  'installed.shownOf': { one: 'Showing {shown} of {count} game', other: 'Showing {shown} of {count} games' },
  // 7-C2 installed
  'installed.storageLoading': 'Reading the disk…',
  'installed.meterDisk': 'Disk {volume}',
  'installed.meterMoreGames': { one: '{count} more game', other: '{count} more games' },
  'installed.moving': 'Moving',
  'installed.menuMoving': '{action}: a move is in progress',
  // /7-C2 installed
};
