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
 * - one hint is shown at a time in the window: a hint opening hides the one shown, which forgets the
 *   pointer; when the last one opened goes, the hint of the control that still has keyboard focus
 *   comes back. Menus are not hints: a hint neither closes a menu nor is closed by one (MenuList keeps
 *   menus to one at a time on its own).
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
  destroy(): void;
  readonly open: boolean;
}

export interface HoverIntentOptions {
  delay?: number;
  grace?: number;
}

interface Hint {
  /** Shown, or hidden only because another opened: either way it gives way to a newer one. */
  readonly holding: boolean;
  /** Hidden because `other` opened: forget the pointer, wait for `other` to go. */
  give(other: Hint): void;
  /** `other`, which took over from this one or from the one this gave way to, went. */
  release(other: Hint): void;
}

// Every hint alive in the window, for the one-at-a-time rule.
const hints = new Set<Hint>();

export function createHoverIntent(onchange: (open: boolean) => void, options: HoverIntentOptions = {}): HoverIntent {
  const { delay = HOVER_DELAY, grace = LEAVE_GRACE } = options;
  let showTimer: ReturnType<typeof setTimeout> | null = null;
  let leaveTimer: ReturnType<typeof setTimeout> | null = null;
  let hovered = false;
  let focused = false;
  let dismissed = false;
  let open = false;
  // The hint this one gave way to, while it waits for it to go.
  let gaveWay: Hint | null = null;

  const cancel = () => {
    if (showTimer !== null) { clearTimeout(showTimer); showTimer = null; }
    if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; }
  };
  const update = () => {
    const next = (hovered || focused) && !dismissed && gaveWay === null;
    if (next === open) return;
    open = next;
    onchange(next);
    // onchange may have closed it again (a control found unusable): only a hint still shown takes over.
    if (next && open) takeOver();
    if (!next && !open) giveBack();
  };
  const self: Hint = {
    get holding() {
      return open || gaveWay !== null;
    },
    give(other) {
      gaveWay = other;
      if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; }
      hovered = false;
      update();
    },
    release(other) {
      if (gaveWay !== other) return;
      gaveWay = null;
      update();
    },
  };
  // A hint waiting on another now waits on this one, so that only the last to go gives focus its hint
  // back. One still waiting out its delay is left to it: it takes over in turn when it shows.
  const takeOver = () => {
    for (const other of [...hints]) if (other !== self && other.holding) other.give(self);
  };
  const giveBack = () => {
    for (const other of [...hints]) if (other !== self) other.release(self);
  };
  hints.add(self);
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

  return {
    hover() {
      if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; return; }
      if (hovered || showTimer !== null) return;
      showTimer = setTimeout(() => { showTimer = null; hovered = true; gaveWay = null; update(); }, delay);
    },
    unhover() {
      if (showTimer !== null) { clearTimeout(showTimer); showTimer = null; }
      if (!hovered) { settle(); return; }
      if (leaveTimer !== null) return;
      if (grace > 0) leaveTimer = setTimeout(leave, grace);
      else leave();
    },
    focus() {
      focused = true;
      gaveWay = null;
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
    },
    destroy() {
      cancel();
      hints.delete(self);
      gaveWay = null;
      if (open) {
        open = false;
        giveBack();
      }
    },
    get open() {
      return open;
    },
  };
}
