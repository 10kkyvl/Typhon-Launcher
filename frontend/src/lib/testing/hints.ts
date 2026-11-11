import { fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';

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

/** The shown tooltip's text after `element` gets keyboard focus, or null when none shows. */
export async function hintOnFocus(element: HTMLElement): Promise<string | null> {
  element.focus();
  // jsdom's focus() does not dispatch focusin, which a browser does.
  await fireEvent.focusIn(element);
  await tick();
  const bubble = document.querySelector('[data-part="tooltip"]');
  const text = bubble?.textContent?.trim() ?? null;
  element.blur();
  await fireEvent.focusOut(element);
  await tick();
  return text;
}
