import type { Message } from '../../types';
import { format } from './format';
import { friends } from './friends';
import { profile } from './profile';
import { search } from './search';
import { install } from './install';
import { downloads } from './downloads';
import { installed } from './installed';
import { settings } from './settings';

export const ru = {
  ...format,
  ...friends,
  ...profile,
  ...search,
  ...install,
  ...downloads,
  ...installed,
  ...settings,
} as const satisfies Record<string, Message>;

export type MessageKey = keyof typeof ru;
