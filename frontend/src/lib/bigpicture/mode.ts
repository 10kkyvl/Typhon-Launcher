import { Events, Window } from '@wailsio/runtime';
import { readonly, writable, type Readable } from 'svelte/store';
import { inWails } from '../services/backend';

const WINDOW_TRANSITION_TIMEOUT_MS = 1500;
const WINDOW_FULLSCREEN_EVENT = 'common:WindowFullscreen';
const WINDOW_UNFULLSCREEN_EVENT = 'common:WindowUnFullscreen';

/**
 * Whether the shell is currently in Big Picture mode.
 *
 * In a browser this describes the shell mode only. Native fullscreen is
 * deliberately best-effort at the boundary of the Wails runtime and is not
 * claimed by the browser preview.
 */
const activeState = writable(false);
export const bigPictureActive: Readable<boolean> = readonly(activeState);

interface EntryWindowState {
  /** Whether the window was already fullscreen before entering the mode. */
  wasFullscreen: boolean;
  /** Whether the window was maximised before entering the mode. */
  wasMaximised: boolean;
  /** Whether the mode's fullscreen transition has already been undone. */
  fullscreenRestored: boolean;
}

let activeValue = false;
let entryWindowState: EntryWindowState | null = null;

// Wails calls are asynchronous. Keeping all lifecycle work on one promise
// chain makes rapid enter/exit requests deterministic and lets an exit queued
// behind an in-flight entry undo that entry safely.
let lifecycleQueue: Promise<void> = Promise.resolve();

// A restore request can be waiting on a native call while exitBigPicture is
// requested. Bump this before queuing the exit so the restore checks it again
// after every await and cannot focus a window after the mode has been exited.
let lifecycleRevision = 0;

function enqueue<T>(operation: () => Promise<T>): Promise<T> {
  const next = lifecycleQueue.then(operation, operation);
  lifecycleQueue = next.then(
    () => undefined,
    () => undefined,
  );
  return next;
}

function setActive(value: boolean) {
  activeValue = value;
  activeState.set(value);
}

interface WindowTransitionWait {
  promise: Promise<void>;
  cancel: () => void;
}

/**
 * Wails' macOS fullscreen calls return when the toggle is queued on the main
 * queue. The common window events are emitted after AppKit finishes the
 * transition. Waiting for that event prevents an exit queued immediately
 * after entry from toggling the same native window twice. The timeout keeps
 * the lifecycle bounded when a platform or custom event mapping does not
 * provide the event.
 */
function waitForWindowTransition(eventName: string): WindowTransitionWait | null {
  const once = Events?.Once;
  if (typeof once !== 'function') return null;

  let resolveWait!: () => void;
  let settled = false;
  let cleaned = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let unsubscribe: (() => void) | undefined;

  const cleanup = () => {
    if (cleaned) return;
    cleaned = true;
    if (timer !== undefined) clearTimeout(timer);
    unsubscribe?.();
  };

  const finish = () => {
    if (settled) return;
    settled = true;
    cleanup();
    resolveWait();
  };

  const promise = new Promise<void>((resolve) => {
    resolveWait = resolve;
  });

  timer = setTimeout(finish, WINDOW_TRANSITION_TIMEOUT_MS);

  try {
    unsubscribe = once(eventName, () => finish());
    // A test adapter or a custom runtime may invoke the callback while
    // registering it. In that case cleanup ran before the unsubscribe handle
    // existed, so remove the handle as soon as registration returns.
    if (cleaned) unsubscribe?.();
  } catch {
    // Event observation is an optional synchronization aid. If the runtime
    // cannot register it, use the bounded native-call result instead.
    cleanup();
    return null;
  }

  return { promise, cancel: finish };
}

async function runWindowTransition(
  operation: () => Promise<void>,
  eventName: string,
  expectedFullscreen: boolean,
): Promise<void> {
  const transition = waitForWindowTransition(eventName);
  try {
    await operation();
    if (transition) await transition.promise;

    // The timeout is deliberately a fallback for runtimes that do not emit a
    // completion event. Do not advertise or discard the mode snapshot unless
    // the native state agrees with the transition we requested.
    if ((await Window.IsFullscreen()) !== expectedFullscreen) {
      throw new Error('native fullscreen transition did not reach the requested state');
    }
  } finally {
    transition?.cancel();
  }
}

/**
 * Enters Big Picture mode.
 *
 * Native mode uses Wails fullscreen. The active store is changed only after
 * all required native calls succeed, so a failed entry leaves the shell able
 * to retry without advertising a mode that was never established.
 */
export function enterBigPicture(): Promise<void> {
  lifecycleRevision += 1;
  return enqueue(async () => {
    if (activeValue) return;

    if (!inWails) {
      entryWindowState = null;
      setActive(true);
      return;
    }

    const [wasFullscreen, wasMaximised] = await Promise.all([
      Window.IsFullscreen(),
      Window.IsMaximised(),
    ]);

    if (!wasFullscreen) {
      await runWindowTransition(
        () => Window.Fullscreen(),
        WINDOW_FULLSCREEN_EVENT,
        true,
      );
    }

    // Wails keeps the previous placement and size while entering fullscreen.
    // The flags are retained so exit can repair a platform that does not
    // restore the maximised state along with fullscreen.
    entryWindowState = {
      wasFullscreen,
      wasMaximised,
      fullscreenRestored: wasFullscreen,
    };
    setActive(true);
  });
}

/**
 * Exits Big Picture mode and restores the window state captured on entry.
 *
 * A failed native call intentionally leaves the active store and snapshot in
 * place; the caller can retry exit after the transient runtime failure.
 */
export function exitBigPicture(): Promise<void> {
  lifecycleRevision += 1;
  return enqueue(async () => {
    if (!activeValue) return;

    if (!inWails) {
      entryWindowState = null;
      setActive(false);
      return;
    }

    const state = entryWindowState;
    if (!state) {
      // This should only be reachable after an unexpected module state change,
      // but it is safer to keep the public store recoverable than to leave it
      // permanently active without a native snapshot.
      setActive(false);
      return;
    }

    // Do not unfullscreen a window that was already fullscreen before the
    // mode was entered. Wails' UnFullscreen restores its saved bounds and
    // size constraints; the maximised check below covers runtimes that do
    // not carry that bit across the transition.
    if (!state.wasFullscreen && !state.fullscreenRestored) {
      await runWindowTransition(
        () => Window.UnFullscreen(),
        WINDOW_UNFULLSCREEN_EVENT,
        false,
      );

      // Keep this phase across a later repair failure. Wails itself guards
      // UnFullscreen with IsFullscreen, but retaining the completed phase
      // also prevents a retry from asking a platform to toggle back into
      // fullscreen after the first transition already finished.
      state.fullscreenRestored = true;
    }

    if (!state.wasFullscreen) {
      const isMaximised = await Window.IsMaximised();
      if (isMaximised !== state.wasMaximised) {
        if (state.wasMaximised) await Window.Maximise();
        else await Window.UnMaximise();
      }
    }

    entryWindowState = null;
    setActive(false);
  });
}

/**
 * Makes an active Big Picture window visible and focused after the game owns
 * the foreground session again.
 *
 * The revision check is intentionally repeated after each await. If exit was
 * requested while a native restore call was pending, the rest of that restore
 * is abandoned and cannot steal focus from the user's next window.
 */
export function restoreBigPictureWindow(): Promise<void> {
  const revision = lifecycleRevision;
  return enqueue(async () => {
    if (!inWails || !activeValue || revision !== lifecycleRevision) return;

    await Window.Show();
    if (!inWails || !activeValue || revision !== lifecycleRevision) return;

    await Window.UnMinimise();
    if (!inWails || !activeValue || revision !== lifecycleRevision) return;

    await Window.Focus();
  });
}
