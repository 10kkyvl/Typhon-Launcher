import type { Message } from '../../types';
import type { MessageKey } from '../ru';
import { format } from './format';
import { friends } from './friends';
import { profile } from './profile';
import { search } from './search';
import { install } from './install';
import { downloads } from './downloads';
import { installed } from './installed';
import { settings } from './settings';

export const en: Record<MessageKey, Message> = {
  ...format,
  ...friends,
  ...profile,
  ...search,
  ...install,
  ...downloads,
  ...installed,
  ...settings,
};
