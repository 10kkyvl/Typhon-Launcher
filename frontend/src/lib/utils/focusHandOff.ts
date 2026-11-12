import { tick } from 'svelte';
import { focusQuietly } from '../layers/focus';

/**
 * For a control that goes away when it is pressed: a button its own action replaces, the last "Load
 * more". Call it at the press, before anything changes; call what it answers once the page shows what
 * came instead. Focus moves to the target only when something held focus at the press and that focus
 * has since fallen to the page. A press that held no focus leaves focus alone (a pointer in the macOS
 * WebKit view, where a click does not focus a button), and so does a control that is still there and a
 * user who has moved on meanwhile.
 *
 * `leaving`: what is on its way out with the pressed control still inside it (a toast fading away);
 * focus in there counts as fallen.
 */
export function focusHandOff(
  leaving?: () => Element | null | undefined,
): (target: () => HTMLElement | null | undefined) => Promise<void> {
  const held = document.activeElement;
  const had = held instanceof HTMLElement && held !== document.body;
  return async (target) => {
    if (!had) return;
    await tick();
    const active = document.activeElement;
    const fallen = !active || active === document.body || !active.isConnected || Boolean(leaving?.()?.contains(active));
    if (!fallen) return;
    const next = target();
    if (next?.isConnected) focusQuietly(next);
  };
}
