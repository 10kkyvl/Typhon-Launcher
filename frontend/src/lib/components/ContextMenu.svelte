<script lang="ts">
  import { flushSync, untrack } from 'svelte';
  import type { ContextPlace } from '../utils/contextmenu';
  import { anchorTo, outOfSight, placeAtPoint } from '../utils/position';
  import MenuList from './MenuList.svelte';
  import { closesOnPick, type MenuFocus, type MenuItem } from './menu';

  /**
   * A context menu (WAI-ARIA menu), mounted while open: MenuList on the layer stack, so Escape, a press
   * outside and a dialog opening over it close it, and focus goes back when it does. Opened at the
   * pointer it sits at that point and closes when the page scrolls, the window resizes or loses focus;
   * opened next to an element (a keyboard open, a menu button) it follows that element and closes when
   * the element leaves sight, which is first brought into view. A pick closes the menu before
   * `onselect` runs, so focus is back where the menu came from and a dialog the pick opens returns
   * focus there; an independent check (`MenuItem.independent`) keeps it open.
   */
  let {
    items,
    id,
    label,
    at,
    initial = 'menu',
    trigger,
    returnFocus,
    onselect,
    onclose,
  }: {
    items: MenuItem[];
    id: string;
    /** The menu's name (what it acts on). */
    label: string;
    at: ContextPlace;
    initial?: MenuFocus;
    /** The menu button it was opened from, if any: a press on it is its own, and focus returns to it. */
    trigger?: () => HTMLElement | null | undefined;
    /** Where focus goes on close when what had it before the menu opened is gone; default the trigger. */
    returnFocus?: () => HTMLElement | null | undefined;
    onselect: (id: string, item: MenuItem) => void;
    /** Unmount the menu. */
    onclose: () => void;
  } = $props();

  // Taken once: the menu is mounted per opening, and the layer asks these as it closes, when the
  // parent's state for this opening may already be gone.
  const { triggerOf, homeOf } = untrack(() => ({ triggerOf: trigger, homeOf: returnFocus }));
  const owner = () => triggerOf?.() ?? null;
  const home = () => homeOf?.() ?? owner();

  // Unmounts the menu now, so the layer stack has put focus back when this returns. Only from event
  // handlers: a dismissal from the stack runs inside effects, where flushSync may not.
  function close() {
    onclose();
    flushSync();
  }

  function pick(item: MenuItem) {
    // Read before the menu goes: the parent's handler may be bound to what the menu was open for.
    const select = onselect;
    if (closesOnPick(item)) close();
    select(item.id, item);
  }

  const place = (node: HTMLElement) => {
    const where = untrack(() => at);
    if (!('anchor' in where)) return placeAtPoint(node, where.x, where.y, { onleave: onclose });
    const anchor = () => (typeof where.anchor === 'function' ? where.anchor() : where.anchor);
    // A focused control scrolled out of sight would close its menu at once as hidden.
    const element = anchor();
    if (element && outOfSight(element)) element.scrollIntoView({ block: 'nearest' });
    return anchorTo(node, anchor, { placement: 'bottom-start', offset: 4, onhide: onclose });
  };
</script>

<MenuList
  {items}
  {id}
  {label}
  {initial}
  {owner}
  returnFocus={home}
  {place}
  onpick={pick}
  onclose={(reason) => (reason === 'tab' ? close() : onclose())}
/>
