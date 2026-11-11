<script lang="ts" module>
  /** What the trigger element needs: spread it on the button (`<Button {...props}>`), wire nothing by hand. */
  export interface MenuTriggerProps {
    id: string;
    'aria-haspopup': 'menu';
    'aria-expanded': boolean;
    'aria-controls': string | undefined;
    onclick: (event: MouseEvent) => void;
    onkeydown: (event: KeyboardEvent) => void;
    onkeyup: (event: KeyboardEvent) => void;
  }
</script>

<script lang="ts">
  import { flushSync, type Snippet } from 'svelte';
  import { anchorTo, type Placement } from '../utils/position';
  import MenuList from './MenuList.svelte';
  import { closesOnPick, type MenuItem } from './menu';

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
  let initial = $state<'first' | 'last' | 'menu'>('menu');

  const side = $derived<Placement>(`${placement === 'up' ? 'top' : 'bottom'}-${align === 'left' ? 'start' : 'end'}`);

  // Looked up each time: a trigger re-created while the menu is open (a re-keyed row) still counts.
  const triggerElement = () => document.getElementById(triggerId);

  function show(focus: 'first' | 'last' | 'menu') {
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

  const triggerProps: MenuTriggerProps = $derived({
    id: triggerId,
    'aria-haspopup': 'menu',
    'aria-expanded': open,
    'aria-controls': open ? menuId : undefined,
    onclick: (event: MouseEvent) => {
      if (open) close();
      // detail 0: a click from script (Big Picture's confirm), treated as the keyboard.
      else show(event.detail === 0 ? 'first' : 'menu');
    },
    // Enter and Space act on keydown, never on the browser's click: a held key repeats keydown (and
    // Enter its click), and the Space keyup after a pick would land on the trigger and reopen.
    onkeydown: (event: KeyboardEvent) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        if (event.repeat) return;
        if (open) close();
        else show('first');
        return;
      }
      if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
      event.preventDefault();
      if (event.repeat && open) return;
      close();
      show(event.key === 'ArrowDown' ? 'first' : 'last');
    },
    onkeyup: (event: KeyboardEvent) => {
      if (event.key === ' ') event.preventDefault();
    },
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
