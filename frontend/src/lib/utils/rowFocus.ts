import { focusHandOff } from './focusHandOff';

/**
 * One rule for focus when a row of a list leaves while focus is in it (a removal, a row Go dropped, a
 * row a filter no longer shows): the same control of the row now in its place, else of the row above;
 * else what the screen names. Read where focus sits before the rows change; hand it on after.
 */

/** Where a row stood: its key, the key of the part of the page it is in, that part's rows in order, the kind of control focus was on. */
export interface RowPlace {
  id: string;
  /** Null on a screen whose rows form one list. */
  scope: string | null;
  order: string[];
  control: string | null;
}

/** What a screen gives the rule. */
export interface RowRule {
  /** The attribute every row carries with its key. */
  rowAttr: string;
  /** The attribute a row's controls carry with their kind (`data-row-control` unless named). */
  controlAttr?: string;
  /**
   * The key of the part of the page `row` belongs to; a row the screen answers null for is in none of
   * them, and focus in it is left alone. Without it the screen's rows form one list.
   */
  scopeOf?(row: Element): string | null;
  /** The part of the page `scope` names, whose rows are the list a row's neighbours are read from. */
  scopeBy?(scope: string | null): Element | null;
  /** The control of `row` of kind `kind`, else the one the screen takes for any. */
  controlIn(row: HTMLElement, kind: string | null): HTMLElement | null;
  /** What focus on `focused` stands for on the page (an open menu's item stands for its trigger). */
  standsFor?(focused: Element): Element;
  /** Where focus goes when no row of the list is left. */
  fallback(place: RowPlace): HTMLElement | null;
}

/** The ids to try after `id` left `order`: the rows below it, then the rows above it, nearest first. */
export function nearOrder(order: readonly string[], id: string): string[] {
  const at = order.indexOf(id);
  return at < 0 ? [] : [...order.slice(at + 1), ...order.slice(0, at).reverse()];
}

/**
 * What focus on an item of an open menu (a layer apart from the page) stands for: the menu's trigger,
 * so the row the menu belongs to is known while it is open. Any other element stands for itself.
 */
export function menuOwner(focused: Element): Element {
  const menu = focused.closest('[role="menu"][aria-labelledby]');
  const trigger = menu ? document.getElementById(menu.getAttribute('aria-labelledby') ?? '') : null;
  return trigger ?? focused;
}

export function rowFocus(rule: RowRule) {
  const controlAttr = rule.controlAttr ?? 'data-row-control';
  const rowId = (row: Element) => row.getAttribute(rule.rowAttr) ?? '';

  function rowsIn(scope: string | null): HTMLElement[] {
    const root: ParentNode | null = rule.scopeBy ? rule.scopeBy(scope) : document;
    return root ? Array.from(root.querySelectorAll<HTMLElement>(`[${rule.rowAttr}]`)) : [];
  }

  /** The place of the row `element` is in (a control in it, or what stands for one), if any. */
  function placeOf(element: Element | null): RowPlace | null {
    if (!element) return null;
    const owner = rule.standsFor ? rule.standsFor(element) : element;
    const row = owner.closest(`[${rule.rowAttr}]`);
    if (!row) return null;
    const scope = rule.scopeOf ? rule.scopeOf(row) : null;
    if (rule.scopeOf && scope === null) return null;
    return {
      id: rowId(row),
      scope,
      order: rowsIn(scope).map(rowId),
      control: owner.closest(`[${controlAttr}]`)?.getAttribute(controlAttr) ?? null,
    };
  }

  const shownIn = (scope: string | null) => new Map(rowsIn(scope).map((row) => [rowId(row), row]));

  /** The row that stood at `place` when it is still shown in its list. */
  function stays(place: RowPlace): HTMLElement | null {
    return shownIn(place.scope).get(place.id) ?? null;
  }

  /** The nearest row of the list that is still shown: below first, then above. */
  function nearest(place: RowPlace): HTMLElement | null {
    const shown = shownIn(place.scope);
    for (const id of nearOrder(place.order, place.id)) {
      const row = shown.get(id);
      if (row) return row;
    }
    return null;
  }

  /** Where focus goes for a row that stood at `place`, by the rule. */
  function leaveTarget(place: RowPlace): HTMLElement | null {
    const row = stays(place) ?? nearest(place);
    return row ? rule.controlIn(row, place.control) : rule.fallback(place);
  }

  /** Brings the row `target` is in into view by the least movement: focus moved by script does not scroll. */
  function reveal(target: HTMLElement | null): void {
    const box = target?.closest<HTMLElement>(`[${rule.rowAttr}]`) ?? target;
    if (box && typeof box.scrollIntoView === 'function') box.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }

  /** Answers `target`, and brings its row into view once whoever focuses it has (in the same task). */
  function inView(target: HTMLElement | null): HTMLElement | null {
    queueMicrotask(() => {
      if (target && document.activeElement === target) reveal(target);
    });
    return target;
  }

  /**
   * The page's catch-all, run before every change of the rows it shows ($effect.pre). When focus is in a
   * row, and the focused element is gone once the change shows, focus goes on by the rule. Focus
   * elsewhere, a dialog's included, is left alone.
   */
  function catchLeaving(): void {
    const place = placeOf(document.activeElement);
    if (!place) return;
    const handOff = focusHandOff();
    void handOff(() => inView(leaveTarget(place)));
  }

  /** For a dialog's `returnFocus`: the rule's target for `place` once the dialog closes. */
  function returnTo(place: RowPlace | null): HTMLElement | null {
    return place ? inView(leaveTarget(place)) : null;
  }

  return { placeOf, stays, nearest, leaveTarget, reveal, catchLeaving, returnTo };
}
