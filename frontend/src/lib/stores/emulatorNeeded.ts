import { Events } from '@wailsio/runtime';
import { derived, writable } from 'svelte/store';
import { TakePending } from '../../../bindings/typhon/emulatorpromptservice';
import type { EmulatorNeeded } from '../../../bindings/typhon/models';
import { errorCode, msg } from '../i18n';
import { libraryErrorText } from '../library/errors';
import { inWails } from '../services/backend';
import { launchGame, playGameProfile } from '../services/library';
import { askForEmulator } from './emulatorPrompt';
import { libraryLoaded } from './library';
import { toast } from './toasts';

// The window can put a question to the player: past the sign-in, legal and
// consent screens (App.svelte's linksReady) and with the library loaded, which
// the conflict dialog looks the game up in.
const signedIn = writable(false);
const windowReady = derived([signedIn, libraryLoaded], ([$signedIn, $loaded]) => $signedIn && $loaded);

// Drops the held-back launch still waiting for the window; a newer launch
// taken from the slot replaces it, whatever its kind.
let dropWaiting: (() => void) | null = null;

function waitForWindow(): Promise<boolean> {
  return new Promise((resolve) => {
    let settled = false;
    let stop = () => {};
    const settle = (go: boolean) => {
      if (settled) return;
      settled = true;
      if (dropWaiting === drop) dropWaiting = null;
      stop();
      resolve(go);
    };
    const drop = () => settle(false);
    dropWaiting = drop;
    stop = windowReady.subscribe((ready) => {
      if (ready) settle(true);
    });
    if (settled) stop();
  });
}

function showLaunchError(err: unknown) {
  toast(libraryErrorText(err, msg('games.errorPlayFailed')), 'danger');
}

// The same launch the tray or the shortcut made, through the in-window path:
// its answer is the current state, handled as for Play pressed in the window.
async function resumeHeldLaunch(held: EmulatorNeeded): Promise<void> {
  if (!(await waitForWindow())) return;
  try {
    // An empty profile from a shortcut is the game's own executable, not the last profile used.
    if (held.profileLaunch) await playGameProfile(held.gameId, held.profileId ?? '');
    else await launchGame(held.gameId);
  } catch (err) {
    if (errorCode(err) === 'library.already_running') return;
    console.error('resume a held-back launch', err);
    showLaunchError(err);
  }
}

export async function takeEmulatorNeeded(): Promise<void> {
  try {
    const pending = await TakePending();
    if (!pending?.gameId) return;
    dropWaiting?.();
    if (!pending.platform) {
      await resumeHeldLaunch(pending);
      return;
    }
    if (!(await askForEmulator(pending.platform, pending.code))) return;
    await launchGame(pending.gameId);
  } catch (err) {
    console.error('emulator needed', err);
    showLaunchError(err);
  }
}

export function setResumeReady(next: boolean) {
  signedIn.set(next);
}

// An emulator prompt is asked at once, before the window is ready, as it
// always was; a launch held back by the cloud saves waits for the window.
export function initEmulatorNeeded() {
  if (!inWails) return () => {};
  void takeEmulatorNeeded();
  const off = Events.On('emulators:needed', () => void takeEmulatorNeeded());
  return () => {
    off();
    dropWaiting?.();
  };
}
