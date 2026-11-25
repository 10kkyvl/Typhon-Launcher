import { rowFocus, type RowPlace } from '../../lib/utils/rowFocus';

export type { RowPlace } from '../../lib/utils/rowFocus';

/**
 * The disk usage screen's rule for focus when a row goes (a delete or a removal that worked): the
 * shared row rule within the row's section; with no row of the section left to take it, the first
 * control left in the section, else the page heading.
 */

/** The attribute every row carries with its key, `<section>:<item>` (UsageRow.svelte). */
export const ROW_ATTR = 'data-usage-row';
/** The attribute a row's control carries with its kind: `delete`, `move`, `remove`. */
export const CONTROL_ATTR = 'data-row-control';

/** A control a press can land on: not disabled (a control off with a reason stays reachable). */
const usable = (el: HTMLElement) => !el.matches(':disabled');

function sectionBy(heading: string | null): HTMLElement | null {
  return heading ? (document.getElementById(heading)?.closest('section') ?? null) : null;
}

function controlIn(row: HTMLElement, kind: string | null): HTMLElement | null {
  const all = Array.from(row.querySelectorAll<HTMLElement>(`[${CONTROL_ATTR}]`)).filter(usable);
  return all.find((el) => el.getAttribute(CONTROL_ATTR) === kind) ?? all[0] ?? null;
}

const rule = rowFocus({
  rowAttr: ROW_ATTR,
  controlAttr: CONTROL_ATTR,
  scopeOf: (row) => row.closest('section[aria-labelledby]')?.getAttribute('aria-labelledby') ?? null,
  scopeBy: sectionBy,
  controlIn,
  fallback: (place) => {
    const section = sectionBy(place.scope);
    const first = section ? Array.from(section.querySelectorAll<HTMLElement>('button, a[href]')).find(usable) : undefined;
    return first ?? document.querySelector<HTMLElement>('h1[tabindex="-1"]');
  },
});

function rowByKey(key: string): HTMLElement | null {
  return Array.from(document.querySelectorAll<HTMLElement>(`[${ROW_ATTR}]`)).find((row) => row.getAttribute(ROW_ATTR) === key) ?? null;
}

/** The place of the row `key`, read now, before the press changes anything; `control` is the kind pressed. */
export function placeOf(key: string, control: string): RowPlace {
  const row = rowByKey(key);
  const place = row ? rule.placeOf(row) : null;
  return place ? { ...place, control } : { id: key, scope: null, order: [], control };
}

/** Where focus goes for a row that stood at `place`, by the screen's rule. */
export function leaveTarget(place: RowPlace): HTMLElement | null {
  const own = rule.stays(place);
  return own ? controlIn(own, place.control) : rule.leaveTarget(place);
}

/**
 * For a dialog's `returnFocus` (a delete's confirmation, the removal dialog): answers the rule's target
 * for `place` once the dialog closes, and brings its row into view after the dialog has focused it
 * (focus moved by script does not scroll).
 */
export function returnTo(place: RowPlace | null): HTMLElement | null {
  if (!place) return null;
  const target = leaveTarget(place);
  queueMicrotask(() => {
    if (target && document.activeElement === target) rule.reveal(target);
  });
  return target;
}
