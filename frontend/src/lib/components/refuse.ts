import type { Attachment } from 'svelte/attachments';

// Only the keys that press a button are held back. Focus stays on a refused or busy button, so every
// other key (Tab, Escape, the arrows, the window's shortcuts) must pass.
const ACTIVATION_KEYS = new Set(['Enter', ' ']);
// A menu button also opens on these (WAI-ARIA menu button).
const POPUP_KEYS = new Set(['ArrowDown', 'ArrowUp']);
const ACTIVATIONS = ['click', 'pointerdown', 'pointerup', 'mousedown', 'mouseup', 'keydown', 'keyup'];

/**
 * While `blocked()` holds, the button ignores every activation: a press, Enter, Space, a form submit,
 * and on a menu button the arrows that open its menu. Capture on the button itself runs before any
 * handler on it or above it (Svelte's delegated ones included), so nothing spread onto it sees a
 * blocked activation; a capture or touch handler a caller adds is not covered. Every other key passes.
 */
export function refuseWhile(blocked: () => boolean): Attachment<HTMLButtonElement> {
  return (node) => {
    const stop = (event: Event) => {
      if (!blocked()) return;
      if (event instanceof KeyboardEvent && !ACTIVATION_KEYS.has(event.key)) {
        if (!POPUP_KEYS.has(event.key) || !node.hasAttribute('aria-haspopup')) return;
      }
      if (event.type !== 'pointerdown' && event.type !== 'mousedown') event.preventDefault();
      event.stopImmediatePropagation();
    };
    for (const type of ACTIVATIONS) node.addEventListener(type, stop, true);
    return () => {
      for (const type of ACTIVATIONS) node.removeEventListener(type, stop, true);
    };
  };
}
