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
 *   hover and focus: the next hover or focus starts afresh.
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

export function createHoverIntent(onchange: (open: boolean) => void, options: HoverIntentOptions = {}): HoverIntent {
  const { delay = HOVER_DELAY, grace = LEAVE_GRACE } = options;
  let showTimer: ReturnType<typeof setTimeout> | null = null;
  let leaveTimer: ReturnType<typeof setTimeout> | null = null;
  let hovered = false;
  let focused = false;
  let dismissed = false;
  let open = false;

  const cancel = () => {
    if (showTimer !== null) { clearTimeout(showTimer); showTimer = null; }
    if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; }
  };
  const update = () => {
    const next = (hovered || focused) && !dismissed;
    if (next === open) return;
    open = next;
    onchange(next);
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

  return {
    hover() {
      if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; return; }
      if (hovered || showTimer !== null) return;
      showTimer = setTimeout(() => { showTimer = null; hovered = true; update(); }, delay);
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
    },
    get open() {
      return open;
    },
  };
}
