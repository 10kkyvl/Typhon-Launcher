/**
 * When a hint (a tooltip, the hover card of a friend row) is shown, as a pure state machine with
 * timers; the components feed it events and render `open`. One rule for every hint:
 *
 * - the pointer resting on the control shows it after `delay`; leaving earlier cancels;
 * - keyboard focus shows it at once and blur hides it (the caller passes only `:focus-visible`);
 * - the pointer may leave for `grace` without hiding it, long enough to cross onto the hint, where
 *   `hover()` keeps it (WCAG 1.4.13: hoverable);
 * - `escape()` (Escape, a press on the control, a dialog over it) hides it until both the pointer and
 *   focus have left, so it does not come back while the user is still there; a press before the hint
 *   has shown counts too (focus then leaving for what the press opened does not bring it up);
 * - `dismiss()` (scroll, the window losing focus, the control gone or disabled) hides it and forgets
 *   hover and focus: the next hover or focus starts afresh;
 * - one hint is shown at a time in the window: a hint opening sets aside the one shown, which forgets
 *   the pointer. Once no hint is shown and the pointer is not resting towards one (crossing from one
 *   hinted control to the next), every hint set aside is let go, and the one whose control still has
 *   keyboard focus shows again. Which hint is shown is one value, not the order hints were
 *   made in. Menus are not hints: a hint neither closes a menu nor is closed by one (MenuList keeps
 *   menus to one at a time on its own);
 * - `mute(true)`: the hint has nothing to show (an empty text): it never opens and takes part in
 *   nothing, neither setting aside the hint shown nor holding the keyboard's hint back while the
 *   pointer rests on its control. It keeps following the pointer and focus, so it shows at once if
 *   it gets something to show while the user is there.
 */

export const HOVER_DELAY = 350;
export const LEAVE_GRACE = 120;

export interface HoverIntent {
  hover(): void;
  unhover(): void;
  focus(): void;
  blur(): void;
  escape(): void;
  dismiss(): void;
  mute(muted: boolean): void;
  destroy(): void;
  readonly open: boolean;
}

export interface HoverIntentOptions {
  delay?: number;
  grace?: number;
}

interface Hint {
  /** The pointer rests on its control and its delay is running: it is about to show. */
  readonly resting: boolean;
  /** Hidden because another hint opened over it: forget the pointer, wait to be let go. */
  setAside(): void;
  /** No hint is shown any more: show again if keyboard focus is still on the control. */
  letGo(): void;
}

// Every hint alive in the window, and the one shown.
const hints = new Set<Hint>();
let shown: Hint | null = null;

function handBack() {
  if (shown !== null) return;
  for (const hint of hints) if (hint.resting) return;
  for (const hint of [...hints]) hint.letGo();
}

export function createHoverIntent(onchange: (open: boolean) => void, options: HoverIntentOptions = {}): HoverIntent {
  const { delay = HOVER_DELAY, grace = LEAVE_GRACE } = options;
  let showTimer: ReturnType<typeof setTimeout> | null = null;
  let leaveTimer: ReturnType<typeof setTimeout> | null = null;
  let hovered = false;
  let focused = false;
  let dismissed = false;
  let open = false;
  let asideNow = false;
  let muted = false;

  const cancel = () => {
    if (showTimer !== null) { clearTimeout(showTimer); showTimer = null; }
    if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; }
  };
  const self: Hint = {
    get resting() {
      return showTimer !== null && !muted;
    },
    setAside() {
      asideNow = true;
      if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; }
      hovered = false;
      update();
    },
    letGo() {
      if (!asideNow) return;
      asideNow = false;
      update();
    },
  };
  const update = () => {
    const next = (hovered || focused) && !dismissed && !asideNow && !muted;
    if (next === open) return;
    open = next;
    if (next) {
      const before = shown;
      shown = self;
      if (before !== null && before !== self) before.setAside();
    } else if (shown === self) {
      shown = null;
    }
    onchange(next);
    if (!next) handBack();
  };
  // The dismissal lasts while the user is still there: hovering, focused, or resting towards a show.
  const settle = () => {
    if (!hovered && !focused && showTimer === null) dismissed = false;
    update();
  };
  const leave = () => {
    leaveTimer = null;
    hovered = false;
    settle();
  };
  hints.add(self);

  return {
    hover() {
      if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; return; }
      if (hovered || showTimer !== null) return;
      showTimer = setTimeout(() => {
        showTimer = null;
        hovered = true;
        asideNow = false;
        update();
        handBack();
      }, delay);
    },
    unhover() {
      if (showTimer !== null) {
        clearTimeout(showTimer);
        showTimer = null;
        handBack();
      }
      if (!hovered) { settle(); return; }
      if (leaveTimer !== null) return;
      if (grace > 0) leaveTimer = setTimeout(leave, grace);
      else leave();
    },
    focus() {
      focused = true;
      asideNow = false;
      update();
    },
    blur() {
      focused = false;
      settle();
    },
    escape() {
      dismissed = true;
      update();
    },
    dismiss() {
      cancel();
      hovered = false;
      focused = false;
      dismissed = true;
      update();
      dismissed = false;
      handBack();
    },
    mute(value) {
      if (value === muted) return;
      muted = value;
      update();
      handBack();
    },
    destroy() {
      cancel();
      hints.delete(self);
      asideNow = false;
      if (open) {
        open = false;
        if (shown === self) shown = null;
      }
      handBack();
    },
    get open() {
      return open;
    },
  };
}
