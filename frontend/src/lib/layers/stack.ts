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
  /** The layer this one belongs to (non-modal layers only). */
  owner?: E | null;
  /** The element it was opened from (its trigger): a press on it is the trigger's, not an outside press. */
  anchor?: F | null;
  /** A press outside this layer and the layers it owns closes it (non-modal layers only). */
  outsidePress?: boolean;
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

  top(): E | null {
    return this.#entries.at(-1)?.element ?? null;
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
      if (entry.owner === undefined || entry.owner === null || !owners.has(entry.owner)) continue;
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
   * Closing a layer under the top one moves its own return target to the layers above it that were
   * opened from inside it, and leaves focus alone.
   */
  close(id: symbol): Closed<E, F> | null {
    const index = this.#entries.findIndex((entry) => entry.id === id);
    if (index < 0) return null;
    const wasTop = index === this.#entries.length - 1;
    const [entry] = this.#entries.splice(index, 1);
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
    const focus = [entry.opener, entry.returnFocus?.(), ...entry.inherited].find(reachable) ?? null;
    return { focus, enter: focus === null ? (modal?.element ?? null) : null };
  }

  /** Escape: the top layer closes if it may; a layer that may not still swallows the key. */
  dismissTop(): Dismissal<E, F> {
    const top = this.#entries.at(-1);
    if (!top) return { consumed: false, closed: null, dismiss: null };
    if (!top.dismissible()) return { consumed: true, closed: null, dismiss: null };
    return { consumed: true, closed: this.close(top.id), dismiss: top.dismiss };
  }

  /**
   * A press on `target`: when the top layer is a non-modal one that closes on an outside press and
   * `target` is outside it and the layers it owns, it closes as with Escape. Null when the press
   * belongs to the layers.
   */
  pressOutside(target: F): Dismissal<E, F> | null {
    const top = this.#entries.at(-1);
    if (!top || top.modal || !top.outsidePress || this.#pressedIn(top, target)) return null;
    return this.dismissTop();
  }

  // A press belongs to a layer when it lands in it, on its trigger, or in a layer it owns (or on that one's trigger).
  #pressedIn(entry: Entry<E, F>, target: F): boolean {
    const owners = new Set([entry.element]);
    for (const candidate of this.#entries) {
      const mine = candidate === entry || (candidate.owner !== undefined && candidate.owner !== null && owners.has(candidate.owner));
      if (!mine) continue;
      owners.add(candidate.element);
      if (this.env.contains(candidate.element, target)) return true;
      if (candidate.anchor && this.env.holds(candidate.anchor, target)) return true;
    }
    return false;
  }

  #topModal(): Entry<E, F> | undefined {
    return this.#entries.findLast((entry) => entry.modal);
  }
}
