import { rowFocus } from '../../lib/utils/rowFocus';

export type { RowPlace } from '../../lib/utils/rowFocus';

/**
 * The LAN screen's rule for focus when a row leaves while focus is in it (a share stopped, an offer Go
 * no longer knows, a transfer that ended): the shared row rule within the row's section; with no row of
 * the section left, the section's first control, else the page heading.
 */

/** The attribute every row carries with its key: an offer's computer and infohash, a game's id, a transfer's id. */
export const ROW_ATTR = 'data-lan-row';
/** The attribute every control of a row carries with its kind. */
export const CONTROL_ATTR = 'data-row-control';
/** The attribute of each of the screen's sections, with its name. */
export const SECTION_ATTR = 'data-lan-section';

function sectionBy(name: string | null): HTMLElement | null {
  return Array.from(document.querySelectorAll<HTMLElement>(`[${SECTION_ATTR}]`)).find((el) => el.getAttribute(SECTION_ATTR) === name) ?? null;
}

export const { placeOf, leaveTarget, catchLeaving, returnTo } = rowFocus({
  rowAttr: ROW_ATTR,
  controlAttr: CONTROL_ATTR,
  scopeOf: (row) => {
    const section = row.closest(`[${SECTION_ATTR}]`);
    return section ? (section.getAttribute(SECTION_ATTR) ?? '') : null;
  },
  scopeBy: sectionBy,
  controlIn: (row, kind) => {
    const marked = Array.from(row.querySelectorAll<HTMLElement>(`[${CONTROL_ATTR}]`));
    return marked.find((el) => el.getAttribute(CONTROL_ATTR) === kind) ?? row.querySelector<HTMLElement>('button, a[href]');
  },
  fallback: (place) => sectionBy(place.scope)?.querySelector<HTMLElement>('button, a[href]') ?? document.querySelector<HTMLElement>('h1[tabindex="-1"]'),
});
