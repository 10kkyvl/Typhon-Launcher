import type { Message } from '../../types';
import type { InstalledKey } from '../ru/installed';

export const installed: Record<InstalledKey, Message> = {
  'installed.youHave': { one: 'You have {count} game installed', other: 'You have {count} games installed' },
  'installed.shownOf': { one: 'Showing {shown} of {count} game', other: 'Showing {shown} of {count} games' },
};
