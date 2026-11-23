import { focusHandOff } from '../../lib/utils/focusHandOff';

/**
 * The Sources screen's rule for focus when a row leaves while focus is in it (a removal, a source Go
 * drops, a row the status filter no longer shows): the same control of the row now in its place, else
 * of the row above; else Add. The rule of the Downloads screen (routes/downloads/rowFocus.ts), whose
 * file is tied to its sections, its panels and its heading fallback, so this one follows it rather than
 * importing it.
 */

/** The attribute every row carries with its source's id. */
export const ROW_ATTR = 'data-source-row';
/** The attribute every control of a row carries with its kind; `open` is the row's lead. */
export const CONTROL_ATTR = 'data-row-control';
/** The attribute of the screen's Add, where focus goes when no row is left. */
export const ADD_ATTR = 'data-sources-add';

/** Where a row stood: its source, the rows shown in order, the kind of control focus was on. */
export interface RowPlace {
  id: string;
  order: string[];
  control: string | null;
}

const rowId = (row: Element) => row.getAttribute(ROW_ATTR) ?? '';
const rows = () => Array.from(document.querySelectorAll<HTMLElement>(`[${ROW_ATTR}]`));

function controlIn(row: HTMLElement, kind: string | null): HTMLElement | null {
  const all = Array.from(row.querySelectorAll<HTMLElement>(`[${CONTROL_ATTR}]`));
  return all.find((el) => el.getAttribute(CONTROL_ATTR) === kind) ?? all.find((el) => el.getAttribute(CONTROL_ATTR) === 'open') ?? all[0] ?? null;
}

/**
 * What focus stands for on the page: the focused element, or, for an item of an open menu (a layer
 * apart from the page), the menu's trigger, so the row the menu belongs to is known while it is open.
 */
function standsFor(focused: Element): Element {
  const menu = focused.closest('[role="menu"][aria-labelledby]');
  const trigger = menu ? document.getElementById(menu.getAttribute('aria-labelledby') ?? '') : null;
  return trigger ?? focused;
}

/** The place of the row `element` (a control in it, or an item of its open menu) is in, if any. */
export function placeOf(element: Element | null): RowPlace | null {
  if (!element) return null;
  const owner = standsFor(element);
  const row = owner.closest(`[${ROW_ATTR}]`);
  if (!row) return null;
  return {
    id: rowId(row),
    order: rows().map(rowId),
    control: owner.closest(`[${CONTROL_ATTR}]`)?.getAttribute(CONTROL_ATTR) ?? null,
  };
}

/** Where focus goes for a row that stood at `place`, by the screen's rule. */
export function leaveTarget(place: RowPlace): HTMLElement | null {
  const shown = new Map(rows().map((row) => [rowId(row), row]));
  const own = shown.get(place.id);
  if (own) return controlIn(own, place.control);
  const at = place.order.indexOf(place.id);
  const near = at < 0 ? [] : [...place.order.slice(at + 1), ...place.order.slice(0, at).reverse()];
  for (const id of near) {
    const row = shown.get(id);
    if (row) return controlIn(row, place.control);
  }
  return document.querySelector<HTMLElement>(`[${ADD_ATTR}]`);
}

/** Brings the row `target` is in into view by the least movement: focus moved by script does not scroll. */
export function reveal(target: HTMLElement | null): void {
  const box = target?.closest<HTMLElement>(`[${ROW_ATTR}]`) ?? target;
  if (box && typeof box.scrollIntoView === 'function') box.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}

/**
 * The page's catch-all, run before every change of the rows it shows ($effect.pre). When focus is in a
 * row, or in the open menu of a row, and the focused element is gone once the change shows, focus goes
 * on by the rule. Focus elsewhere, a dialog's included, is left alone.
 */
export function catchLeaving(): void {
  const place = placeOf(document.activeElement);
  if (!place) return;
  const handOff = focusHandOff();
  void handOff(() => {
    const target = leaveTarget(place);
    queueMicrotask(() => {
      if (target && document.activeElement === target) reveal(target);
    });
    return target;
  });
}

/**
 * For a dialog's `returnFocus` (the removal's confirmation): answers the rule's target for `place`
 * once the dialog closes, and brings its row into view after the dialog has focused it.
 */
export function returnTo(place: RowPlace | null): HTMLElement | null {
  if (!place) return null;
  const target = leaveTarget(place);
  queueMicrotask(() => {
    if (target && document.activeElement === target) reveal(target);
  });
  return target;
}
