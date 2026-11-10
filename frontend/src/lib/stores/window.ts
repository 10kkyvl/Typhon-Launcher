import { Events } from '@wailsio/runtime';
import { derived, writable, type Readable } from 'svelte/store';
import { msg } from '../i18n';
import { inWails } from '../services/backend';
import { appInfo } from '../services/system';
import { isWindowMaximised, windowAction, type WindowAction } from '../services/window';
import { toast } from './toasts';

/**
 * Whether macOS draws its own traffic lights in the title bar instead of the DOM buttons. The window
 * is frameless on every OS, so the native buttons exist only once `main.go` asks for them; until then
 * macOS gets the DOM buttons like every other platform.
 */
export const MAC_NATIVE_WINDOW_CONTROLS = false;

export type WindowControls = 'buttons' | 'native';

export function windowControlsFor(platform: string, macNative: boolean = MAC_NATIVE_WINDOW_CONTROLS): WindowControls {
  return platform === 'darwin' && macNative ? 'native' : 'buttons';
}

const platformStore = writable('unknown');

/** Go's `runtime.GOOS`, `'browser'` outside Wails, `'unknown'` until `initPlatform` or after it failed. */
export const platform: Readable<string> = { subscribe: platformStore.subscribe };

export const windowControls: Readable<WindowControls> = derived(platform, ($platform) => windowControlsFor($platform));

export async function initPlatform(): Promise<void> {
  try {
    platformStore.set((await appInfo()).platform);
  } catch (err) {
    console.error('read platform', err);
  }
}

const maximised = writable(false);

export const windowMaximised: Readable<boolean> = { subscribe: maximised.subscribe };

export function initWindowState(): () => void {
  if (!inWails) return () => {};
  let generation = 0;
  const set = (value: boolean) => {
    generation++;
    maximised.set(value);
  };
  const refresh = () => {
    const asked = ++generation;
    isWindowMaximised()
      .then((value) => {
        if (asked === generation) maximised.set(value);
      })
      .catch((err: unknown) => console.error('read window maximised state', err));
  };
  const { Common, Windows, Mac } = Events.Types;
  const listen = (names: string[], handler: () => void) => names.map((name) => Events.On(name, handler));
  // Windows reports a restore on every resize step, so restore and unmaximise set the state
  // directly; only leaving fullscreen, which may land maximised, asks the window again.
  const offs = [
    ...listen([Common.WindowMaximise, Windows.WindowMaximise, Mac.WindowMaximise], () => set(true)),
    ...listen(
      [Common.WindowUnMaximise, Common.WindowRestore, Windows.WindowUnMaximise, Windows.WindowRestore, Mac.WindowUnMaximise],
      () => set(false),
    ),
    ...listen(
      [
        Common.WindowFullscreen,
        Common.WindowUnFullscreen,
        Windows.WindowFullscreen,
        Windows.WindowUnFullscreen,
        Mac.WindowDidEnterFullScreen,
        Mac.WindowDidExitFullScreen,
      ],
      refresh,
    ),
  ];
  refresh();
  return () => {
    for (const off of offs) off();
  };
}

// A double-click on the caption toggles maximise, as on a native title bar;
// one that lands on a caption button belongs to that button.
export function captionDoubleClick(target: EventTarget | null): void {
  const element = target as { closest?: (selector: string) => unknown } | null;
  if (element?.closest?.('.no-drag')) return;
  // runWindowAction reports its own failures and never rejects.
  void runWindowAction('toggleMaximise');
}

export async function runWindowAction(action: WindowAction): Promise<void> {
  if (!inWails) {
    toast(msg('ui.desktopOnly'));
    return;
  }
  try {
    await windowAction(action);
  } catch (err) {
    console.error(`window ${action}`, err);
    toast(msg('ui.windowActionFailed'), 'danger');
  }
}
