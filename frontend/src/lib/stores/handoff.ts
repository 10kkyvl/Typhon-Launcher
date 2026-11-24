import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { EVENT_HANDOFF_DONE, getHandoffInfo, handoffRendered, type HandoffInfo } from '../services/selfupdate';
import { announceUpdated } from './selfupdate';
import { releaseShow, takeOverShow } from './updateShow';

/**
 * The successor's side of a seamless restart. Go starts it hidden beside the old launcher, holds its
 * services back until the old one has exited, and shows its window in the old one's place once told
 * the first frame is on screen (HandoffRendered). In order:
 *
 * 1. `loadHandoff` (main.ts, before the app mounts): Go says whether this launcher is a successor;
 *    if so the show is taken over on the old launcher's clock and held.
 * 2. `curtainDrawn` (UpdateShow, every frame drawn with the curtain closed): after two more animation
 *    frames, so that frame is really on screen, Go is told once.
 * 3. The show is released when both have happened: Go says the old launcher is gone and the
 *    held-back services have started (`launcher:handoff_done`), and the shell has its first data
 *    (`setShellReady`, App.svelte). Then the show plays its finish and the result is said.
 *
 * Nothing here keeps time for truth: a successor whose old launcher never exits is ended by Go
 * (CompleteHandoff gives up and quits), and one that never reports is rolled back by the old one.
 */

let successor: HandoffInfo | null = null;
let reported = false;
let oldGone = false;
let shellReady = false;
let offDone: (() => void) | null = null;

type Frame = (next: () => void) => unknown;

export async function loadHandoff(): Promise<HandoffInfo | null> {
  if (!inWails) return null;
  stopListening();
  successor = null;
  reported = false;
  oldGone = false;
  // Listen before asking: Go may finish between the answer and a subscription made after it.
  offDone = Events.On(EVENT_HANDOFF_DONE, () => {
    oldGone = true;
    settle();
  });
  let info: HandoffInfo;
  try {
    info = await getHandoffInfo();
  } catch (err) {
    // Without Go's answer this start is an ordinary one: a successor that cannot tell is never shown
    // and the old launcher rolls back when its time is up.
    console.error('read the update hand-over', err);
    stopListening();
    return null;
  }
  if (!info.active) {
    stopListening();
    return null;
  }
  successor = info;
  takeOverShow(info.phaseStart, info.version);
  return info;
}

export function curtainDrawn(frame: Frame = (next) => requestAnimationFrame(next)): void {
  if (!successor || reported) return;
  reported = true;
  frame(() =>
    frame(() => {
      handoffRendered().catch((err: unknown) => {
        // The window is still hidden, so there is no one to tell; Go's own time limit rolls back.
        console.error('report the first frame of the update hand-over', err);
      });
    }),
  );
}

export function setShellReady(ready: boolean): void {
  shellReady = ready;
  settle();
}

function settle() {
  if (!successor || !oldGone || !shellReady) return;
  const { version } = successor;
  successor = null;
  stopListening();
  announceUpdated(version);
  releaseShow();
}

function stopListening() {
  offDone?.();
  offDone = null;
}
