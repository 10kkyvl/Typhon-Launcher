import { menuOwner, rowFocus } from '../../lib/utils/rowFocus';

export type { RowPlace } from '../../lib/utils/rowFocus';

/**
 * The friends screen's rule for focus when a card or a row leaves while focus is in it (a request
 * answered or cancelled, a player unblocked, a friend removed or blocked): the shared row rule within
 * its section (incoming, outgoing, friends, blocked); with no row of the section left, the section's
 * first control, else the chosen tab, else the page heading. An open row menu stands for its row.
 */

/** The attribute every card and row carries with its user's id. */
export const ROW_ATTR = 'data-friends-row';
/** The attribute every control of a card or a row carries with its kind. */
export const CONTROL_ATTR = 'data-row-control';
/** The attribute of each section of the screen, with its name. */
export const SECTION_ATTR = 'data-friends-section';

function sectionBy(name: string | null): HTMLElement | null {
  return Array.from(document.querySelectorAll<HTMLElement>(`[${SECTION_ATTR}]`)).find((el) => el.getAttribute(SECTION_ATTR) === name) ?? null;
}

export const { placeOf, catchLeaving, returnTo } = rowFocus({
  rowAttr: ROW_ATTR,
  controlAttr: CONTROL_ATTR,
  scopeOf: (row) => {
    const section = row.closest(`[${SECTION_ATTR}]`);
    return section ? (section.getAttribute(SECTION_ATTR) ?? '') : null;
  },
  scopeBy: sectionBy,
  controlIn: (row, kind) => {
    const marked = Array.from(row.querySelectorAll<HTMLElement>(`[${CONTROL_ATTR}]`));
    return marked.find((el) => el.getAttribute(CONTROL_ATTR) === kind) ?? row.querySelector<HTMLElement>('button:not(:disabled), a[href]');
  },
  standsFor: menuOwner,
  fallback: (place) =>
    sectionBy(place.scope)?.querySelector<HTMLElement>('button:not(:disabled), a[href], input') ??
    document.querySelector<HTMLElement>('[role="radio"][aria-checked="true"]') ??
    document.querySelector<HTMLElement>('h1[tabindex="-1"]'),
});
