import { menuOwner, rowFocus } from '../../lib/utils/rowFocus';

export type { RowPlace } from '../../lib/utils/rowFocus';

/**
 * The Sources screen's rule for focus when a row leaves while focus is in it, or in the row's open menu
 * (a removal, a source Go drops, a row the status filter no longer shows): the shared row rule over the
 * screen's one list; with no row left, Add.
 */

/** The attribute every row carries with its source's id. */
export const ROW_ATTR = 'data-source-row';
/** The attribute every control of a row carries with its kind; `open` is the row's lead. */
export const CONTROL_ATTR = 'data-row-control';
/** The attribute of the screen's Add, where focus goes when no row is left. */
export const ADD_ATTR = 'data-sources-add';

export const { placeOf, leaveTarget, reveal, catchLeaving, returnTo } = rowFocus({
  rowAttr: ROW_ATTR,
  controlAttr: CONTROL_ATTR,
  standsFor: menuOwner,
  controlIn: (row, kind) => {
    const all = Array.from(row.querySelectorAll<HTMLElement>(`[${CONTROL_ATTR}]`));
    return all.find((el) => el.getAttribute(CONTROL_ATTR) === kind) ?? all.find((el) => el.getAttribute(CONTROL_ATTR) === 'open') ?? all[0] ?? null;
  },
  fallback: () => document.querySelector<HTMLElement>(`[${ADD_ATTR}]`),
});
