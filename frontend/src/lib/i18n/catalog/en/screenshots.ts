import type { Message } from '../../types';
import type { ScreenshotsKey } from '../ru/screenshots';

export const screenshots: Record<ScreenshotsKey, Message> = {
  'settings.screenshotsEnabledLabel': 'Screenshots',
  'settings.screenshotsEnabledSub': 'Save a frame of the game to the Screenshots folder with a hotkey',
  'settings.screenshotHotkeyLabel': 'Screenshot key',
  'settings.screenshotHotkeySub': 'Press it in a game to take a screenshot',
  'settings.screenshotsSaveError': 'Could not apply the screenshot setting: {reason}',
  'settings.screenshotsSaveErrorPlain': 'Could not apply the screenshot setting',
  'screenshots.saved': 'Screenshot saved: {name}',
  'screenshots.failed': 'Could not save the screenshot: {reason}',
  'screenshots.failedPlain': 'Could not save the screenshot',
  'screenshots.sectionTitle': 'My screenshots',
  'screenshots.openFolder': 'Open folder',
  'screenshots.delete': 'Delete screenshot',
  'screenshots.deleteConfirm': 'The screenshot "{name}" will be deleted from disk and cannot be restored.',
  'screenshots.deleted': 'Screenshot deleted',
  'screenshots.open': 'Open screenshot {name}',
  'screenshots.hotkeyHint': 'Take a screenshot with {hotkey}',
  'screenshots.emptyNoHotkey': 'No screenshots yet',
  'screenshots.empty': 'No screenshots yet. Press {hotkey} in the game.',
  'screenshots.disabledNote': 'Screenshots are turned off in settings.',
  'screenshots.noLibrary': 'The screenshots folder is unavailable: set up the library in settings first.',
  'screenshots.loadError': 'Could not load screenshots',
  'screenshots.actionError': 'Could not complete the screenshot action',
  'screenshots.loading': 'Loading screenshots',
};
