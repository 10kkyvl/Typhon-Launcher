import type { Message } from '../../types';
import type { ErrGameModeKey } from '../ru/errGameMode';

export const errGameMode: Record<ErrGameModeKey, Message> = {
  'gamemode.hide_needs_tray': 'Game mode: could not hide the launcher, turn on “Minimize to tray” in settings',
  'gamemode.power_scheme_missing': 'Game mode: Windows has no “High performance” power plan',
  'gamemode.power_failed': 'Game mode: could not switch the power plan',
  'gamemode.priority_failed': 'Game mode: could not raise the game priority. It may be running as administrator',
  'gamemode.unsupported': 'Game mode: this action is not supported on your system',
  'gamemode.marker_write_failed': 'Game mode: could not save the data needed to restore the power plan, the plan was not changed',
  'gamemode.marker_corrupt': 'Game mode: the power plan restore file is damaged, the power plan was not changed',
  'gamemode.restore_failed': 'Game mode: could not restore the previous power plan, check it in Windows settings',
  'gamemode.failed': 'Game mode: could not apply the settings',
};
