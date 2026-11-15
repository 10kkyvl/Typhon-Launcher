import type { Leave, LeaveHow, LeaveTicket } from '../../lib/components/downloadPanel';
import { focusHandOff } from '../../lib/utils/focusHandOff';
import { downloadsSectionIds } from './sections';

/**
 * The downloads screen's one rule for focus when a row or a panel leaves while focus is in it: the
 * download's own row where it moved, when the press asks to follow it (`LeaveHow.follow`); the row now
 * in its place, else the row above; else the section's heading; else the page heading. The next
 * section's first control is never guessed. The parts call `pressed` at a press that may take their
 * row away; the page calls `catchLeaving` before every change of its lists, which covers what goes by
 * Go's own doing (a download completing, failing, starting, a control Go no longer offers).
 */

/** What `focusHandOff` answers: called once the page shows what came instead of the pressed control. */
export type HandOff = ReturnType<typeof focusHandOff>;

/** The attribute every row of the screen, a panel included, carries with its download's id. */
export const ROW_ATTR = 'data-download-row';

// The controls a hand-off lands on, as the parts mark them. A row's first one in the page's order is
// its lead: the grip, the panel's and the completed row's lead button, Retry.
const CONTROLS = ['data-grip', 'data-panel-control', 'data-row-control', 'data-retry', 'data-menu', 'data-cancel'];
const ANY_CONTROL = CONTROLS.map((name) => `[${name}]`).join(', ');

/** The element of `root` whose `data-<name>` is `id` (an id is matched as data, never as a selector). */
export function byData(root: ParentNode | undefined, name: string, id: string): HTMLElement | null {
  if (!root) return null;
  const key = name.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
  return Array.from(root.querySelectorAll<HTMLElement>(`[data-${name}]`)).find((el) => el.dataset[key] === id) ?? null;
}

/** The page's heading, focusable by script: where focus goes when nothing of a section is left to take it. */
export function pageHeading(): HTMLElement | null {
  return document.querySelector<HTMLElement>('h1[tabindex="-1"]');
}

/** Where a row stood: its download, the id of its section's heading, its section's rows in order, the kind of control focus was on. */
interface RowPlace {
  id: string;
  heading: string | null;
  order: string[];
  control: string | null;
}

const rowId = (row: Element) => row.getAttribute(ROW_ATTR) ?? '';

function rowsIn(section: Element | null | undefined): HTMLElement[] {
  return section ? Array.from(section.querySelectorAll<HTMLElement>(`[${ROW_ATTR}]`)) : [];
}

// A panel is a section of its own: its screen section is the nearest one above it.
const sectionOf = (row: Element) => row.parentElement?.closest('section[aria-labelledby]') ?? null;

function sectionBy(heading: string | null): Element | null {
  return heading ? (document.getElementById(heading)?.closest('section') ?? null) : null;
}

/** The control `el` is, as a selector a row of the same part answers: `[data-cancel]`, `[data-panel-control="more"]`. */
function kindOf(el: Element | null): string | null {
  if (!el) return null;
  const name = CONTROLS.find((attr) => el.hasAttribute(attr));
  if (!name) return null;
  return name === 'data-panel-control' || name === 'data-row-control' ? `[${name}="${el.getAttribute(name)}"]` : `[${name}]`;
}

function controlIn(row: HTMLElement, prefer: string | null): HTMLElement | null {
  return (prefer ? row.querySelector<HTMLElement>(prefer) : null) ?? row.querySelector<HTMLElement>(ANY_CONTROL);
}

function placeOf(id: string, focused: Element | null): RowPlace {
  const row = byData(document, 'download-row', id);
  const section = row ? sectionOf(row) : null;
  return {
    id,
    heading: section?.getAttribute('aria-labelledby') ?? null,
    order: rowsIn(section).map(rowId),
    control: focused && row?.contains(focused) ? kindOf(focused) : null,
  };
}

/** Where focus goes for a row that stood at `place`, by the screen's rule. */
function leaveTarget(place: RowPlace, how: LeaveHow = {}): HTMLElement | null {
  const prefer = how.prefer ?? place.control;
  const shown = new Map(rowsIn(sectionBy(place.heading)).map((row) => [rowId(row), row]));
  // The row stayed and only its control went (a button Go no longer offers): its own row keeps focus.
  const own = shown.get(place.id);
  if (own) return controlIn(own, prefer);
  const moved = how.follow ? byData(document, 'download-row', place.id) : null;
  if (moved && how.follow === 'first') return controlIn(moved, null);
  const at = place.order.indexOf(place.id);
  const near = at < 0 ? [] : [...place.order.slice(at + 1), ...place.order.slice(0, at).reverse()];
  for (const other of near) {
    const row = shown.get(other);
    if (row) return controlIn(row, prefer);
  }
  if (moved) return controlIn(moved, null);
  return headingOrPage(place.heading);
}

function headingOrPage(id: string | null): HTMLElement | null {
  const heading = id ? document.getElementById(id) : null;
  return heading?.isConnected ? heading : pageHeading();
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

// What the presses in flight ask for, by download: the page's catch-all hands focus on as they would.
const intents = new Map<string, LeaveHow>();

/**
 * Called at a press that may take row `id` away (Cancel, Retry, Resume, Start now): reads the row's
 * place now. `handOn` once Go took the action and the row has left; `drop` when Go refused it.
 */
export const pressed: Leave = (id, how = {}): LeaveTicket => {
  const place = placeOf(id, document.activeElement);
  const handOff = focusHandOff();
  intents.set(id, how);
  const forget = () => {
    if (intents.get(id) === how) intents.delete(id);
  };
  return {
    target: () => leaveTarget(place, how),
    handOn: () => {
      forget();
      return handOffInView(handOff, () => leaveTarget(place, how));
    },
    drop: forget,
  };
};

const SECTION_HEADINGS: ReadonlySet<string> = new Set(Object.values(downloadsSectionIds));

/**
 * What focus stands for on the page: the focused element, or, for an item of an open menu (a layer
 * apart from the page), the menu's trigger, so the row the menu belongs to is known while it is open.
 */
function standsFor(focused: HTMLElement): HTMLElement {
  const menu = focused.closest<HTMLElement>('[role="menu"][aria-labelledby]');
  const trigger = menu ? document.getElementById(menu.getAttribute('aria-labelledby') ?? '') : null;
  return trigger ?? focused;
}

/**
 * The page's catch-all, run before every change of its lists ($effect.pre). When the element that holds
 * focus is gone once the change shows, focus goes on by the rule from where it was: in a row (or a menu
 * of a row), the same row's other control while the row stays, else the rule's steps, as the press in
 * flight on that row asked if any; on a section's own control or heading, that heading, else the page
 * heading. Focus that is still there, a dialog's included, and focus outside the screen's sections are
 * left alone.
 */
export function catchLeaving(): void {
  const focused = document.activeElement;
  if (!(focused instanceof HTMLElement) || focused === document.body) return;
  const owner = standsFor(focused);
  const row = owner.closest<HTMLElement>(`[${ROW_ATTR}]`);
  if (!row) {
    const heading = owner.closest('section[aria-labelledby]')?.getAttribute('aria-labelledby');
    if (heading && SECTION_HEADINGS.has(heading)) void handOffInView(focusHandOff(), () => headingOrPage(heading));
    return;
  }
  const place = placeOf(rowId(row), owner);
  // Read now: the press's own hand-off may settle in the same update and forget it.
  const how = intents.get(place.id);
  void handOffInView(focusHandOff(), () => {
    if (how && intents.get(place.id) === how) intents.delete(place.id);
    return leaveTarget(place, how);
  });
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
