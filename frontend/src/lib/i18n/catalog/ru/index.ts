import type { Message } from '../../types';
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
import { errInstall } from './errInstall';
import { errMetadata } from './errMetadata';
import { errUpdates } from './errUpdates';
import { errSources } from './errSources';
import { errLibrary } from './errLibrary';

export const ru = {
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
  ...errInstall,
  ...errMetadata,
  ...errUpdates,
  ...errSources,
  ...errLibrary,
} as const satisfies Record<string, Message>;

export type MessageKey = keyof typeof ru;

export { common, format, friends, profile, search, install, downloads, installed, settings, modals, ui, social, games, transfers, state, errInstall, errMetadata, errUpdates, errSources, errLibrary };
