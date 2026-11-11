<script lang="ts" module>
  /** What a popover's opener needs: spread it on the control (`<IconButton {...props}>`), wire nothing by hand. */
  export interface PopoverTriggerProps {
    id: string;
    'aria-haspopup': 'dialog';
    'aria-expanded': boolean;
    'aria-controls': string | undefined;
    onclick: (event: MouseEvent) => void;
    onkeydown: (event: KeyboardEvent) => void;
  }
</script>

<script lang="ts">
  import { flushSync, type Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { cycleFocus, layer, topModalLayer } from '../layers';
  import { focusableIn, initialFocus } from '../layers/focus';
  import { anchorTo, outOfSight, type Placement } from '../utils/position';

  /**
   * A small non-modal dialog opened by a button (WAI-ARIA dialog without `aria-modal`, disclosure-style
   * trigger). The trigger snippet renders the button and spreads `props` on it:
   *
   *   <Popover label={msg('ui.notifications')} placement="bottom-end">
   *     {#snippet trigger({ props })}
   *       <IconButton label={msg('ui.notifications')} {...props}><Bell /></IconButton>
   *     {/snippet}
   *     {#snippet children({ close })}…{/snippet}
   *   </Popover>
   *
   * It opens next to the trigger, out of the page (a layer owned by the trigger, see lib/layers), with
   * focus on its first control or on itself when it has none; it follows the trigger on scroll and
   * resize and closes when the trigger scrolls out of sight. The stack answers Escape, presses outside
   * and a dialog opening. Its controls follow the trigger in Tab order: Tab past the last one closes it
   * and focus moves on from the trigger, Shift+Tab before the first closes it onto the trigger. Arrow
   * keys are left to its content. `close` from the content puts focus back on the trigger.
   */
  let {
    open = $bindable(false),
    label,
    placement = 'bottom-start',
    width,
    partId,
    trigger,
    children,
  }: {
    /** Bind it only to close the popover from outside (a route change). */
    open?: boolean;
    /** The dialog's accessible name. */
    label: string;
    placement?: Placement;
    /** A CSS width; by default the content's own. */
    width?: string;
    /** `data-id` next to `data-part="popover"`, for themes. */
    partId?: string;
    trigger: Snippet<[{ open: boolean; toggle: () => void; props: PopoverTriggerProps }]>;
    children: Snippet<[{ close: () => void }]>;
  } = $props();

  const uid = $props.id();
  const triggerId = `${uid}-trigger`;
  const panelId = `${uid}-panel`;

  // Looked up each time: a trigger re-created while the popover is open (a re-keyed row) still counts.
  const triggerElement = () => document.getElementById(triggerId);

  function show() {
    // A trigger scrolled out of sight can still hold focus; its popover would close at once as hidden.
    const element = triggerElement();
    if (element && outOfSight(element)) element.scrollIntoView({ block: 'nearest' });
    open = true;
  }

  // Unmounts the panel now, so the layer stack has put focus back on the trigger when this returns.
  // Only from event handlers: a dismissal from the stack runs inside effects, where flushSync may not.
  function close() {
    if (!open) return;
    open = false;
    flushSync();
  }

  function toggle() {
    if (open) close();
    else show();
  }

  const triggerProps: PopoverTriggerProps = $derived({
    id: triggerId,
    'aria-haspopup': 'dialog',
    'aria-expanded': open,
    'aria-controls': open ? panelId : undefined,
    onclick: () => toggle(),
    // Enter and Space act on keydown, never on the browser's click: a held key repeats keydown (and
    // Enter its click). Space prevented on keydown presses nothing when it comes up inside the panel.
    onkeydown: (event) => {
      if (event.key !== 'Enter' && event.key !== ' ') return;
      event.preventDefault();
      if (!event.repeat) toggle();
    },
  });

  const entry = () => {
    const panel = document.getElementById(panelId);
    return panel ? initialFocus(panel) : null;
  };

  const editable = (target: EventTarget | null) =>
    target instanceof HTMLElement && (target.isContentEditable || target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement);

  function tabOut(event: KeyboardEvent) {
    close();
    if (event.shiftKey) {
      event.preventDefault();
      return;
    }
    // On the page the browser moves on from the trigger; a dialog keeps Tab inside itself.
    const dialog = topModalLayer();
    if (!dialog) return;
    event.preventDefault();
    cycleFocus(dialog, false);
  }

  // Capture: the popover's keys are its own before the layer's handler sees Tab.
  function onKeydown(event: KeyboardEvent) {
    if (event.isComposing) return;
    if (event.key === 'Enter' || event.key === ' ') {
      // A key held since the trigger opened the popover presses nothing in it.
      if (event.repeat && !editable(event.target)) event.preventDefault();
      return;
    }
    if (event.key !== 'Tab' || event.ctrlKey || event.metaKey || event.altKey) return;
    const panel = event.currentTarget as HTMLElement;
    const list = focusableIn(panel);
    const active = document.activeElement;
    const leaving = event.shiftKey ? active === panel || active === list[0] : list.length === 0 || active === list.at(-1);
    if (leaving) tabOut(event);
  }

  // A control removed while it has focus (a list item that went away) hands focus to the panel rather
  // than drop it to <body>, where Escape and Tab would no longer reach the popover.
  const keepFocus: Attachment<HTMLElement> = (node) => {
    // The layer may have focused the panel's first control before this runs.
    let holding = node.contains(document.activeElement);
    const onFocusin = () => (holding = true);
    const onFocusout = (event: FocusEvent) => {
      const to = event.relatedTarget;
      // No relatedTarget: the window lost focus or the focused control was removed; the panel still holds it.
      if (to instanceof Node && !node.contains(to)) holding = false;
    };
    const watcher = new MutationObserver(() => {
      const active = document.activeElement;
      if (holding && (!active || active === document.body || !active.isConnected)) node.focus({ preventScroll: true });
    });
    node.addEventListener('focusin', onFocusin);
    node.addEventListener('focusout', onFocusout);
    watcher.observe(node, { childList: true, subtree: true });
    return () => {
      watcher.disconnect();
      node.removeEventListener('focusin', onFocusin);
      node.removeEventListener('focusout', onFocusout);
    };
  };

  // A trigger scrolled out of sight takes its popover with it; the stack returns focus to the trigger.
  const place: Attachment<HTMLElement> = (node) =>
    anchorTo(node, triggerElement, { placement, offset: 6, onhide: () => (open = false) });
</script>

{@render trigger({ open, toggle, props: triggerProps })}

{#if open}
  <div
    class="popover"
    id={panelId}
    role="dialog"
    aria-label={label}
    tabindex="-1"
    data-part="popover"
    data-id={partId}
    style:width
    onkeydowncapture={onKeydown}
    {@attach layer({ modal: false, owner: triggerElement, returnFocus: triggerElement, initialFocus: entry, dismiss: () => (open = false) })}
    {@attach place}
    {@attach keepFocus}
  >
    {@render children({ close })}
  </div>
{/if}

<style>
  .popover {
    position: fixed;
    z-index: var(--z-layer);
    padding: 0.5rem;
    overflow-y: auto;
    overscroll-behavior: contain;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: popup-in var(--dur-fast) var(--ease);
  }

  /* With no control inside, the panel itself holds focus; its controls show where focus is. */
  .popover:focus-visible {
    outline: none;
  }
</style>
