import type { Message } from '../../types';
import type { OverlayKey } from '../ru/overlay';

export const overlay: Record<OverlayKey, Message> = {
  'overlay.panelLabel': 'Typhon overlay',
  'overlay.hint': '{key} or Esc to return to the game',
  'overlay.tabChat': 'Chat',
  'overlay.tabFriends': 'Friends',
  'overlay.signIn': 'Sign in to your account in the launcher',
  'overlay.accountError': 'Could not load your account. Try again.',
  'overlay.friendsError': 'Could not load friends. Try again.',
  'overlay.hideError': 'Could not close the overlay. Press Esc or the hotkey again.',
  'overlay.consentNeeded': 'Finish setting up your account in the launcher',
  'settings.overlayCardTitle': 'Game overlay',
  'settings.overlayEnabledLabel': 'Overlay over the game',
  'settings.overlayEnabledSub': 'Chat and friends on a hotkey, without leaving the game',
  'settings.overlayExclusiveNote': 'The overlay does not open while a game is in exclusive fullscreen. Switch the game to borderless windowed mode.',
  'settings.overlayHotkeyLabel': 'Hotkey',
  'settings.overlayHotkeySub': 'If another program already uses the key, the overlay will not turn on',
  'settings.overlayStatusLabel': 'Status',
  'settings.overlayStatusUnsupported': 'Windows only for now',
  'settings.overlayStatusOff': 'Off',
  'settings.overlayStatusOn': 'Working: {hotkey}',
  'settings.overlayStatusUnknown': 'Could not read the overlay status',
  'settings.overlaySaveError': 'Could not apply the overlay setting: {reason}',
  'settings.overlaySaveErrorPlain': 'Could not apply the overlay setting',
};
