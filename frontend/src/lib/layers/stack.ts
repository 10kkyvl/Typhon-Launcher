/**
 * The order of open layers (dialogs, the image viewer; later menus, selects, popovers, tooltips) and
 * what follows from it, without a DOM: which layer is on top, what Escape closes, what must be inert,
 * where focus goes when a layer closes. `E` is a layer's root element, `F` a focus target; the DOM side
 * (index.ts) applies the answers.
 *
 * A modal layer makes everything under it inert: the app content and every layer below it. A
 * non-modal layer (a menu inside a dialog) inerts nothing; it is still on the stack, so Escape closes
 * it before the dialog under it. A non-modal layer may name its owner (the layer it was opened from,
 * such as a listbox portalled out of a dialog): it then counts as inside that owner for the Tab trap,
 * for reachability of focus targets and for presses "outside".
 */

export interface LayerSpec<E, F> {
  element: E;
  modal: boolean;
  dismissible: () => boolean;
  dismiss: () => void;
  /** What had focus when the layer opened; focus goes back to it on close. */
  opener: F | null;
  /** Where focus goes when the opener is gone. */
  returnFocus?: () => F | null | undefined;
  /**
   * The layer this one belongs to (non-modal layers only). A getter is read each time it is needed:
   * the trigger it comes from may be re-created while the layer is open.
   */
  owner?: Live<E>;
  /** The element it was opened from (its trigger): a press on it is the trigger's, not an outside press. Read live too. */
  anchor?: Live<F>;
  /**
   * Where focus goes when the opener is gone, ahead of `returnFocus`: the triggers of the menus this
   * dialog closed as it opened from one of their items (see dismissNonModal).
   */
  adopted?: F[];
  /** A press outside this layer and the layers it owns closes it (non-modal layers only). */
  outsidePress?: boolean;
  /**
   * A hint: non-modal, takes no focus, stays open on outside presses. It never stands in the way of
   * the layers under it: presses pass over it, it answers no focus when it closes, and a layer under it
   * closes as if it were the top one.
   */
  passive?: boolean;
}

/** A value, or a getter for one that may change while the layer is open. */
export type Live<T> = T | null | (() => T | null | undefined);

function now<T>(value: Live<T> | undefined): T | null {
  return (typeof value === 'function' ? (value as () => T | null | undefined)() : value) ?? null;
}

interface Entry<E, F> extends LayerSpec<E, F> {
  id: symbol;
  /** Openers of layers that closed under this one while it was open and contained its opener. */
  inherited: F[];
}

/** Focus `focus`; when there is none, enter `enter` (the modal layer on top now); when neither, leave focus alone. */
export interface Closed<E, F> {
  focus: F | null;
  enter: E | null;
}

export interface Dismissal<E, F> {
  /** Escape (or Big Picture's back) belongs to the layers: a layer was open, dismissible or not. */
  consumed: boolean;
  closed: Closed<E, F> | null;
  dismiss: (() => void) | null;
}

export interface StackEnv<E, F> {
  contains(element: E, target: F): boolean;
  /** `target` is `anchor` or inside it. */
  holds(anchor: F, target: F): boolean;
  alive(target: F): boolean;
}

export class LayerStack<E, F> {
  #entries: Entry<E, F>[] = [];

  constructor(private readonly env: StackEnv<E, F>) {}

  get size(): number {
    return this.#entries.length;
  }

  open(spec: LayerSpec<E, F>): symbol {
    const id = Symbol('layer');
    this.#entries.push({ ...spec, id, inherited: [] });
    return id;
  }

  has(id: symbol): boolean {
    return this.#entries.some((entry) => entry.id === id);
  }

  /** Every open layer's element, bottom to top. */
  elements(): E[] {
    return this.#entries.map((entry) => entry.element);
  }

  top(): E | null {
    return this.#entries.at(-1)?.element ?? null;
  }

  /** The element of the topmost layer that is not passive (the hints above it aside), or null. */
  topActive(): E | null {
    return this.#entries.findLast((entry) => !entry.passive)?.element ?? null;
  }

  /** The element of the topmost modal layer: the dialog that owns the keyboard. */
  topModal(): E | null {
    return this.#topModal()?.element ?? null;
  }

  isTop(id: symbol): boolean {
    return this.#entries.at(-1)?.id === id;
  }

  isTopModal(id: symbol): boolean {
    return this.#topModal()?.id === id;
  }

  /** The layer element that contains `target`, if any. */
  holding(target: F): E | null {
    return this.#entries.findLast((entry) => this.env.contains(entry.element, target))?.element ?? null;
  }

  /** The layers `element` owns, directly or through layers it owns, in stack order, with their openers. */
  owned(element: E): { element: E; opener: F | null }[] {
    const owners = new Set([element]);
    const found: { element: E; opener: F | null }[] = [];
    for (const entry of this.#entries) {
      const owner = now(entry.owner);
      if (owner === null || !owners.has(owner)) continue;
      owners.add(entry.element);
      found.push({ element: entry.element, opener: entry.opener });
    }
    return found;
  }

  /** `target` is inside `element` or inside a layer it owns. */
  within(element: E, target: F): boolean {
    return this.env.contains(element, target) || this.owned(element).some((layer) => this.env.contains(layer.element, target));
  }

  /** The modal layer whose Tab trap covers this layer: the top modal, when this is it or a layer it owns. */
  trapFor(id: symbol): E | null {
    const modal = this.#topModal();
    if (!modal) return null;
    if (modal.id === id) return modal.element;
    const entry = this.#entries.find((candidate) => candidate.id === id);
    return entry && this.owned(modal.element).some((layer) => layer.element === entry.element) ? modal.element : null;
  }

  /** What must be inert now: the app content when a modal layer is open, and every layer under the topmost modal. */
  inert(): { app: boolean; layers: E[] } {
    const index = this.#entries.findLastIndex((entry) => entry.modal);
    if (index < 0) return { app: false, layers: [] };
    return { app: true, layers: this.#entries.slice(0, index).map((entry) => entry.element) };
  }

  /**
   * Removes a layer. Closing the top layer answers where focus goes: its opener, else its fallback,
   * else the opener of a layer that closed under it; a target must be reachable (inside the modal layer
   * that is now on top, or anywhere when none is). When nothing is, focus enters the new top modal.
   * Only passive layers above it still count as the top. Closing a layer under the top one moves its
   * own return target to the layers above it that were opened from inside it, and leaves focus alone.
   * A passive layer answers nothing: focus was never its own.
   */
  close(id: symbol): Closed<E, F> | null {
    const index = this.#entries.findIndex((entry) => entry.id === id);
    if (index < 0) return null;
    const wasTop = this.#entries.slice(index + 1).every((above) => above.passive);
    const [entry] = this.#entries.splice(index, 1);
    if (entry.passive) return null;
    if (!wasTop) {
      const home = [entry.opener, ...entry.inherited].filter((target): target is F => target !== null);
      for (const above of this.#entries.slice(index)) {
        if (above.opener !== null && this.env.contains(entry.element, above.opener)) above.inherited.push(...home);
      }
      return null;
    }
    const modal = this.#topModal();
    const reachable = (target: F | null | undefined): target is F =>
      target !== null && target !== undefined && this.env.alive(target) && (!modal || this.within(modal.element, target));
    const focus = [entry.opener, ...(entry.adopted ?? []), entry.returnFocus?.(), ...entry.inherited].find(reachable) ?? null;
    return { focus, enter: focus === null ? (modal?.element ?? null) : null };
  }

  /**
   * A modal layer is opening: every non-modal layer open now (a page menu, a tooltip, a listbox owned
   * by a lower dialog) would be left inert under or over the new scrim, so each that may close is
   * removed; their `dismiss` functions are returned for the caller to run. When the dialog opens from
   * an item of one of them (`opener` inside it), that layer's way home (its trigger, then its opener)
   * is returned as `homes`, for the dialog to adopt: the item is gone when the dialog closes.
   */
  dismissNonModal(opener: F | null = null): { dismiss: (() => void)[]; homes: F[] } {
    const closing = this.#entries.filter((entry) => !entry.modal && entry.dismissible());
    this.#entries = this.#entries.filter((entry) => !closing.includes(entry));
    const homes = closing
      .filter((entry) => opener !== null && this.env.contains(entry.element, opener))
      .reverse()
      .flatMap((entry) => [now(entry.anchor), entry.opener, ...entry.inherited])
      .filter((home): home is F => home !== null && home !== undefined && !closing.some((other) => this.env.contains(other.element, home)));
    return { dismiss: closing.map((entry) => entry.dismiss), homes: [...new Set(homes)] };
  }

  /** Escape: the top layer closes if it may; a layer that may not still swallows the key. */
  dismissTop(): Dismissal<E, F> {
    const top = this.#entries.at(-1);
    if (!top) return { consumed: false, closed: null, dismiss: null };
    if (!top.dismissible()) return { consumed: true, closed: null, dismiss: null };
    return { consumed: true, closed: this.close(top.id), dismiss: top.dismiss };
  }

  /**
   * A press on `target`: non-modal layers on top that close on an outside press close, top first, as
   * with Escape, until one holds the press (in it, on its trigger, in a layer it owns), may not close,
   * or a modal layer or one that stays open is reached. A passive layer (a hint) is passed over, unless
   * the press lands in it: the press is for what lies under it. A press in a parent menu closes its
   * submenu only; a press outside both closes both. Empty when the press belongs to the layers.
   */
  pressOutside(target: F): Dismissal<E, F>[] {
    const closed: Dismissal<E, F>[] = [];
    for (const entry of [...this.#entries].reverse()) {
      if (entry.passive) {
        if (this.env.contains(entry.element, target)) break;
        continue;
      }
      if (entry.modal || !entry.outsidePress || this.#pressedIn(entry, target) || !entry.dismissible()) break;
      closed.push({ consumed: true, closed: this.close(entry.id), dismiss: entry.dismiss });
    }
    return closed;
  }

  // A press belongs to a layer when it lands in it, on its trigger, or in a layer it owns (or on that one's trigger).
  #pressedIn(entry: Entry<E, F>, target: F): boolean {
    const owners = new Set([entry.element]);
    for (const candidate of this.#entries) {
      const owner = now(candidate.owner);
      const mine = candidate === entry || (owner !== null && owners.has(owner));
      if (!mine) continue;
      owners.add(candidate.element);
      if (this.env.contains(candidate.element, target)) return true;
      const anchor = now(candidate.anchor);
      if (anchor !== null && this.env.holds(anchor, target)) return true;
    }
    return false;
  }

  #topModal(): Entry<E, F> | undefined {
    return this.#entries.findLast((entry) => entry.modal);
  }
}
