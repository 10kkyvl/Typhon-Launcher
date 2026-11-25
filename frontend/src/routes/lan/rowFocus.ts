import { focusHandOff } from '../../lib/utils/focusHandOff';

/**
 * The LAN screen's rule for focus when a row leaves while focus is in it (a share stopped, an offer Go
 * no longer knows, a transfer that ended): the same control of the row now in its place, else of the row
 * above; else the section's first control; else the page heading. The Sources screen's rule
 * (routes/sources/rowFocus.ts) with the screen's sections, whose file is tied to its own list and its
 * Add, so this one follows it rather than importing it.
 */

/** The attribute every row carries with its key: an offer's computer and infohash, a game's id, a transfer's id. */
export const ROW_ATTR = 'data-lan-row';
/** The attribute every control of a row carries with its kind. */
export const CONTROL_ATTR = 'data-row-control';
/** The attribute of each of the screen's sections, with its name. */
export const SECTION_ATTR = 'data-lan-section';

/** Where a row stood: its key, its section, the section's rows in order, the kind of control focus was on. */
export interface RowPlace {
  id: string;
  section: string;
  order: string[];
  control: string | null;
}

const rowId = (row: Element) => row.getAttribute(ROW_ATTR) ?? '';

function sectionBy(name: string): HTMLElement | null {
  return Array.from(document.querySelectorAll<HTMLElement>(`[${SECTION_ATTR}]`)).find((el) => el.getAttribute(SECTION_ATTR) === name) ?? null;
}

const rowsIn = (section: Element | null) => (section ? Array.from(section.querySelectorAll<HTMLElement>(`[${ROW_ATTR}]`)) : []);

function controlIn(row: HTMLElement, kind: string | null): HTMLElement | null {
  const marked = Array.from(row.querySelectorAll<HTMLElement>(`[${CONTROL_ATTR}]`));
  return marked.find((el) => el.getAttribute(CONTROL_ATTR) === kind) ?? row.querySelector<HTMLElement>('button, a[href]');
}

/** The place of the row `element` is in, if it is in one of the screen's rows. */
export function placeOf(element: Element | null): RowPlace | null {
  const row = element?.closest(`[${ROW_ATTR}]`);
  const section = row?.closest(`[${SECTION_ATTR}]`);
  if (!element || !row || !section) return null;
  return {
    id: rowId(row),
    section: section.getAttribute(SECTION_ATTR) ?? '',
    order: rowsIn(section).map(rowId),
    control: element.closest(`[${CONTROL_ATTR}]`)?.getAttribute(CONTROL_ATTR) ?? null,
  };
}

/** Where focus goes for a row that stood at `place`, by the screen's rule. */
export function leaveTarget(place: RowPlace): HTMLElement | null {
  const section = sectionBy(place.section);
  const shown = new Map(rowsIn(section).map((row) => [rowId(row), row]));
  const own = shown.get(place.id);
  if (own) return controlIn(own, place.control);
  const at = place.order.indexOf(place.id);
  const near = at < 0 ? [] : [...place.order.slice(at + 1), ...place.order.slice(0, at).reverse()];
  for (const id of near) {
    const row = shown.get(id);
    if (row) return controlIn(row, place.control);
  }
  return section?.querySelector<HTMLElement>('button, a[href]') ?? document.querySelector<HTMLElement>('h1[tabindex="-1"]');
}

/** Brings the row `target` is in into view by the least movement: focus moved by script does not scroll. */
function reveal(target: HTMLElement | null): void {
  const box = target?.closest<HTMLElement>(`[${ROW_ATTR}]`) ?? target;
  if (box && typeof box.scrollIntoView === 'function') box.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}

/**
 * The page's catch-all, run before every change of the rows it shows ($effect.pre). When focus is in a
 * row and the focused element is gone once the change shows, focus goes on by the rule. Focus elsewhere,
 * a dialog's included, is left alone.
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

/** For a dialog's `returnFocus`: the rule's target for `place` once the dialog closes. */
export function returnTo(place: RowPlace | null): HTMLElement | null {
  if (!place) return null;
  const target = leaveTarget(place);
  queueMicrotask(() => {
    if (target && document.activeElement === target) reveal(target);
  });
  return target;
}
