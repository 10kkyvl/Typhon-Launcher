import { tick } from 'svelte';
import { user } from './dom';

// For *.dom.test.ts: whether a hint reaches a user who has no mouse.

/** The text of the elements `element`'s aria-describedby points to. */
export function description(element: Element): string {
  const ids = element.getAttribute('aria-describedby')?.split(/\s+/).filter(Boolean) ?? [];
  return ids.map((id) => element.ownerDocument.getElementById(id)?.textContent?.trim() ?? '').join(' ');
}

/** `element` is in the Tab order: a keyboard user can reach it. */
export function tabbable(element: HTMLElement): boolean {
  return element.tabIndex >= 0 && !(element as HTMLButtonElement).disabled && !element.closest('[hidden], [inert]');
}

/** Every element in `root` that a keyboard user can reach and whose description is `text`. */
export function describedControls(root: ParentNode, text: string): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>('[aria-describedby]')].filter((element) => tabbable(element) && description(element) === text);
}

/**
 * The shown tooltip's text once Tab has brought keyboard focus to `element`, or null when none shows.
 * Focus then moves on with Tab again.
 */
export async function hintOnFocus(element: HTMLElement): Promise<string | null> {
  const keys = user();
  for (let step = 0; step < 300 && document.activeElement !== element; step += 1) await keys.tab();
  if (document.activeElement !== element) throw new Error('Tab never reaches the element');
  await tick();
  const text = document.querySelector('[data-part="tooltip"]')?.textContent?.trim() ?? null;
  await keys.tab();
  return text;
}
