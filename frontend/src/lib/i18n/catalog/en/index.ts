import type { Message } from '../../types';
import type { MessageKey } from '../ru';
import { common } from './common';
import { format } from './format';
import { friends } from './friends';
import { profile } from './profile';
import { search } from './search';
import { install } from './install';
import { downloads } from './downloads';
import { installed } from './installed';
import { settings } from './settings';
import { modals } from './modals';
import { ui } from './ui';
import { social } from './social';
import { games } from './games';
import { transfers } from './transfers';
import { state } from './state';

export const en: Record<MessageKey, Message> = {
  ...common,
  ...format,
  ...friends,
  ...profile,
  ...search,
  ...install,
  ...downloads,
  ...installed,
  ...settings,
  ...modals,
  ...ui,
  ...social,
  ...games,
  ...transfers,
  ...state,
};
