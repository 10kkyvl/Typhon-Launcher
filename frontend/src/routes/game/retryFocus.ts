import { tick } from 'svelte';
import { focusQuietly } from '../../lib/layers/focus';

/**
 * A Retry inside a load-error notice keeps its notice, and the focus, while it is busy. When the answer
 * arrives and takes the notice away with the focused button, focus goes to `target` (the heading or
 * the first control of what loaded) instead of falling to the page.
 */
export async function focusAfterRetry(button: Element | null, target: () => HTMLElement | null | undefined): Promise<void> {
  await tick();
  if (!button || button.isConnected) return;
  const active = document.activeElement;
  if (active && active !== document.body && active.isConnected) return;
  const next = target();
  if (next?.isConnected) focusQuietly(next);
}
