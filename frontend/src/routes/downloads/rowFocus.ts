import type { focusHandOff } from '../../lib/utils/focusHandOff';

/** What `focusHandOff` answers: called once the page shows what came instead of the pressed control. */
export type HandOff = ReturnType<typeof focusHandOff>;

/** The element of `root` whose `data-<name>` is `id` (an id is matched as data, never as a selector). */
export function byData(root: ParentNode | undefined, name: string, id: string): HTMLElement | null {
  if (!root) return null;
  const key = name.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
  return Array.from(root.querySelectorAll<HTMLElement>(`[data-${name}]`)).find((el) => el.dataset[key] === id) ?? null;
}

/** The page's heading, focusable by script: where focus goes when a list has no row left to take it. */
export function pageHeading(): HTMLElement | null {
  return document.querySelector<HTMLElement>('h1[tabindex="-1"]');
}

/**
 * Where focus goes after the row of `id` left `list` from place `index`: `pick` of the row now in its
 * place, else of the row above, else the page heading. The section's own heading takes no focus, and
 * goes with the section once its last row has left.
 */
export function afterLeaving(
  list: HTMLElement | undefined,
  id: string,
  index: number,
  pick: (row: HTMLElement) => HTMLElement | null,
): HTMLElement | null {
  const rows = list ? Array.from(list.querySelectorAll<HTMLElement>(':scope > li')).filter((row) => row.dataset.id !== id) : [];
  const near = rows[index] ?? rows[index - 1] ?? rows.at(-1);
  return (near && pick(near)) ?? pageHeading();
}

/** Finishes a hand-off and, when focus landed on the target, scrolls it into view by the least movement. */
export async function handOffInView(handOff: HandOff, pick: () => HTMLElement | null): Promise<void> {
  const picked: { target: HTMLElement | null } = { target: null };
  await handOff(() => (picked.target = pick()));
  const { target } = picked;
  if (target && document.activeElement === target && typeof target.scrollIntoView === 'function') {
    target.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }
}

/**
 * Things a list waits to see after Go answered, such as a row gone or a button no longer offered: Go's
 * event may come before its answer or after it. `watch` runs `then` at once when `done` already holds,
 * else on the first `check` (called whenever the list's data changed) that finds it holding.
 */
export function createWatchers() {
  const waiting = new Set<{ done: () => boolean; then: () => void }>();
  return {
    watch(done: () => boolean, then: () => void): void {
      if (done()) then();
      else waiting.add({ done, then });
    },
    check(): void {
      for (const watcher of waiting) {
        if (!watcher.done()) continue;
        waiting.delete(watcher);
        watcher.then();
      }
    },
  };
}
