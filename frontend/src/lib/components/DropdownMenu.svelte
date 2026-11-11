<script lang="ts">
  import { flushSync, type Snippet } from 'svelte';
  import { anchorTo, outOfSight, type Placement } from '../utils/position';
  import MenuList from './MenuList.svelte';
  import { closesOnPick, menuButtonHandlers, type MenuFocus, type MenuItem, type MenuTriggerProps } from './menu';

  /**
   * Menu button (WAI-ARIA menu button). The trigger snippet renders the button and spreads `props` on it:
   *
   *   <DropdownMenu {items} onselect={run}>
   *     {#snippet trigger({ props })}
   *       <IconButton label={msg('games.moreLabel')} {...props}><EllipsisVertical /></IconButton>
   *     {/snippet}
   *   </DropdownMenu>
   *
   * The menu opens next to the trigger, out of the page (a layer, see lib/layers), flipping and shifting
   * to stay in the window and following the trigger on scroll and resize; it closes when the trigger
   * scrolls out of sight. A pick closes the menu and
   * puts focus back on the trigger before `onselect` runs, so a dialog the pick opens returns focus
   * there; an independent check (`MenuItem.independent`) keeps the menu open. The open state shows on
   * the trigger as `[aria-expanded="true"]`.
   */
  let {
    items,
    align = 'right',
    placement = 'down',
    onselect,
    trigger,
  }: {
    items: MenuItem[];
    /** Which edge of the trigger the menu lines up with. */
    align?: 'left' | 'right';
    placement?: 'down' | 'up';
    onselect?: (id: string, item: MenuItem) => void;
    trigger: Snippet<[{ open: boolean; toggle: () => void; props: MenuTriggerProps }]>;
  } = $props();

  const uid = $props.id();
  const triggerId = `${uid}-trigger`;
  const menuId = `${uid}-menu`;

  let open = $state(false);
  let initial = $state<MenuFocus>('menu');

  const side = $derived<Placement>(`${placement === 'up' ? 'top' : 'bottom'}-${align === 'left' ? 'start' : 'end'}`);

  // Looked up each time: a trigger re-created while the menu is open (a re-keyed row) still counts.
  const triggerElement = () => document.getElementById(triggerId);

  function show(focus: MenuFocus) {
    // A trigger scrolled out of sight can still hold focus; its menu would close at once as hidden.
    const element = triggerElement();
    if (element && outOfSight(element)) element.scrollIntoView({ block: 'nearest' });
    initial = focus;
    open = true;
  }

  // Unmounts the menu now, so the layer stack has put focus back on the trigger when this returns.
  // Only from event handlers: a dismissal from the stack runs inside effects, where flushSync may not.
  function close() {
    if (!open) return;
    open = false;
    flushSync();
  }

  function toggle() {
    if (open) close();
    else show('menu');
  }

  const handlers = menuButtonHandlers({ open: () => open, show, close });

  const triggerProps: MenuTriggerProps = $derived({
    id: triggerId,
    'aria-haspopup': 'menu',
    'aria-expanded': open,
    'aria-controls': open ? menuId : undefined,
    ...handlers,
  });

  function pick(item: MenuItem) {
    if (closesOnPick(item)) close();
    onselect?.(item.id, item);
  }

  // A trigger scrolled out of sight takes its menu with it: the menu closes rather than stay clamped in
  // the window, detached. The stack returns focus to the trigger without scrolling to it.
  const place = (node: HTMLElement) =>
    anchorTo(node, triggerElement, { placement: side, offset: 4, onhide: () => (open = false) });
</script>

{@render trigger({ open, toggle, props: triggerProps })}

{#if open}
  <MenuList
    {items}
    id={menuId}
    labelledby={triggerId}
    {initial}
    owner={triggerElement}
    {place}
    onpick={pick}
    onclose={(reason) => (reason === 'tab' ? close() : (open = false))}
  />
{/if}
