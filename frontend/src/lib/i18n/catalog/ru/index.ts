import type { Message } from '../../types';
import { common } from './common';
import { bigpictureCatalog } from './bigpictureCatalog';
import { bigpictureTransfers } from './bigpictureTransfers';
import { bigpictureSettings } from './bigpictureSettings';
import { bigpictureSocial } from './bigpictureSocial';
import { bigpicture } from './bigpicture';
import { format } from './format';
import { friends } from './friends';
import { profile } from './profile';
import { profileStyle } from './profileStyle';
import { search } from './search';
import { install } from './install';
import { downloads } from './downloads';
import { installed } from './installed';
import { settings } from './settings';
import { modals } from './modals';
import { ui } from './ui';
import { social } from './social';
import { games } from './games';
import { reviews } from './reviews';
import { transfers } from './transfers';
import { state } from './state';
import { errInstall } from './errInstall';
import { errMetadata } from './errMetadata';
import { errUpdates } from './errUpdates';
import { errSources } from './errSources';
import { errLibrary } from './errLibrary';
import { errLogs } from './errLogs';
import { errSaves } from './errSaves';
import { saves } from './saves';
import { overlay } from './overlay';

export const ru = {
  ...bigpictureCatalog,
  ...bigpictureTransfers,
  ...bigpictureSettings,
  ...bigpictureSocial,
  ...bigpicture,
  ...common,
  ...format,
  ...friends,
  ...profile,
  ...profileStyle,
  ...search,
  ...install,
  ...downloads,
  ...installed,
  ...settings,
  ...modals,
  ...ui,
  ...social,
  ...games,
  ...reviews,
  ...transfers,
  ...state,
  ...errInstall,
  ...errMetadata,
  ...errUpdates,
  ...errSources,
  ...errLibrary,
  ...errLogs,
  ...errSaves,
  ...saves,
  ...overlay,
} as const satisfies Record<string, Message>;

export type MessageKey = keyof typeof ru;

export { common, format, friends, profile, profileStyle, search, install, downloads, installed, settings, modals, ui, social, games, reviews, transfers, state, errInstall, errMetadata, errUpdates, errSources, errLibrary, errLogs, errSaves, saves, overlay };
