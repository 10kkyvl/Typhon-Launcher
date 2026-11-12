import { tick } from 'svelte';
import { focusableIn, focusQuietly } from '../../lib/layers/focus';

/**
 * A load-error notice stays, with its Retry busy, until the answer arrives; when the answer is good the
 * notice goes, and the Retry with it. Focus that was on it moves to the first control of what took the
 * notice's place (`root`), not to the page. Call with the element that had focus when Retry was pressed.
 */
export async function focusAfterRetry(from: Element | null, root: HTMLElement | undefined): Promise<void> {
  await tick();
  if (!from || from.isConnected) return;
  const active = document.activeElement;
  if (active && active !== document.body && active.isConnected) return;
  const target = root ? focusableIn(root)[0] : undefined;
  if (target) focusQuietly(target);
}
