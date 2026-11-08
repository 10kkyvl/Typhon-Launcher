import { Events } from '@wailsio/runtime';
import { TakePending } from '../../../bindings/typhon/emulatorpromptservice';
import { msg } from '../i18n';
import { libraryErrorText } from '../library/errors';
import { inWails } from '../services/backend';
import { launchGame } from '../services/library';
import { askForEmulator } from './emulatorPrompt';
import { toast } from './toasts';

export async function takeEmulatorNeeded(): Promise<void> {
  try {
    const pending = await TakePending();
    if (!pending?.gameId || !pending.platform) return;
    if (!(await askForEmulator(pending.platform, pending.code))) return;
    await launchGame(pending.gameId);
  } catch (err) {
    console.error('emulator needed', err);
    toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
  }
}

export function initEmulatorNeeded() {
  if (!inWails) return () => {};
  void takeEmulatorNeeded();
  return Events.On('emulators:needed', () => void takeEmulatorNeeded());
}
