/**
 * The disk usage screen's rule for focus when a row goes (a delete or a removal that worked): the same
 * control in the row now in its place, else in the row above; else the first control left in the row's
 * section; else the page heading. The rule of the Sources screen (routes/sources/rowFocus.ts), which
 * is tied to its own markers, so this one follows it rather than importing it.
 */

/** The attribute every row carries with its key, `<section>:<item>` (UsageRow.svelte). */
export const ROW_ATTR = 'data-usage-row';
/** The attribute a row's control carries with its kind: `delete`, `move`, `remove`. */
export const CONTROL_ATTR = 'data-row-control';

/** Where a row stood: its key, the keys of its section's rows in order, the kind of control pressed. */
export interface RowPlace {
  key: string;
  section: string | null;
  order: string[];
  control: string;
}

const keyOf = (row: Element) => row.getAttribute(ROW_ATTR) ?? '';

function rowsIn(section: Element | null): HTMLElement[] {
  return section ? Array.from(section.querySelectorAll<HTMLElement>(`[${ROW_ATTR}]`)) : [];
}

function sectionBy(heading: string | null): Element | null {
  return heading ? (document.getElementById(heading)?.closest('section') ?? null) : null;
}

function rowByKey(key: string): HTMLElement | null {
  return Array.from(document.querySelectorAll<HTMLElement>(`[${ROW_ATTR}]`)).find((row) => keyOf(row) === key) ?? null;
}

/** A control a press can land on: not disabled (a control off with a reason stays reachable). */
const usable = (el: HTMLElement) => !el.matches(':disabled');

function controlIn(row: HTMLElement, kind: string): HTMLElement | null {
  const all = Array.from(row.querySelectorAll<HTMLElement>(`[${CONTROL_ATTR}]`)).filter(usable);
  return all.find((el) => el.getAttribute(CONTROL_ATTR) === kind) ?? all[0] ?? null;
}

/** The place of the row `key`, read now, before the press changes anything. */
export function placeOf(key: string, control: string): RowPlace {
  const row = rowByKey(key);
  const section = row?.closest('section[aria-labelledby]') ?? null;
  return { key, section: section?.getAttribute('aria-labelledby') ?? null, order: rowsIn(section).map(keyOf), control };
}

/** Where focus goes for a row that stood at `place`, by the screen's rule. */
export function leaveTarget(place: RowPlace): HTMLElement | null {
  const section = sectionBy(place.section);
  const shown = new Map(rowsIn(section).map((row) => [keyOf(row), row]));
  const own = shown.get(place.key);
  if (own) return controlIn(own, place.control);
  const at = place.order.indexOf(place.key);
  const near = at < 0 ? [] : [...place.order.slice(at + 1), ...place.order.slice(0, at).reverse()];
  for (const key of near) {
    const row = shown.get(key);
    const control = row ? controlIn(row, place.control) : null;
    if (control) return control;
  }
  const first = section ? Array.from(section.querySelectorAll<HTMLElement>('button, a[href]')).find(usable) : undefined;
  return first ?? document.querySelector<HTMLElement>('h1[tabindex="-1"]');
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
    if (!target || document.activeElement !== target) return;
    const box = target.closest<HTMLElement>(`[${ROW_ATTR}]`) ?? target;
    if (typeof box.scrollIntoView === 'function') box.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  });
  return target;
}
