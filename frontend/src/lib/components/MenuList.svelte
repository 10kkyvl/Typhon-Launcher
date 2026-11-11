<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { Check } from '@lucide/svelte';
  import { cycleFocus, layer, topModalLayer } from '../layers';
  import { createTypeahead, isTypeaheadKey, nextIndex } from '../utils/roving';
  import { menuItemRole, type MenuItem } from './menu';

  /**
   * The body of a menu (WAI-ARIA menu): a non-modal layer owned by the element it was opened from,
   * with roving focus, typeahead and Enter/Space activation. DropdownMenu renders it under a trigger;
   * the context menu and the account menu are to reuse it. The layer stack answers Escape, presses
   * outside and a dialog opening over it (`onclose('dismiss')`); Tab closes it (`onclose('tab')`) and
   * focus moves on from the owner, inside the dialog the owner sits in when there is one. Where it
   * appears is the parent's `place` attachment; which items close it on pick is the parent's call.
   */
  let {
    items,
    id,
    labelledby,
    label,
    initial = 'menu',
    owner,
    place,
    onpick,
    onclose,
  }: {
    items: MenuItem[];
    id: string;
    /** Id of the element that names the menu (its trigger); `label` instead when there is none. */
    labelledby?: string;
    label?: string;
    /** Focus on open: the first or last enabled item (opened from the keyboard), or the menu itself (by pointer). */
    initial?: 'first' | 'last' | 'menu';
    /** The element the menu was opened from: a press on it is its own, and focus goes back to it. */
    owner: () => HTMLElement | null | undefined;
    /** Puts the menu on screen (`anchorTo`, a point). */
    place?: Attachment<HTMLElement>;
    onpick: (item: MenuItem) => void;
    /** Close now. On 'tab' focus must be back on the owner when this returns. */
    onclose: (reason: 'dismiss' | 'tab') => void;
  } = $props();

  const typeahead = createTypeahead();
  const isDisabled = (index: number) => Boolean(items[index]?.disabled);

  function itemsIn(root: Element): HTMLElement[] {
    return [...root.querySelectorAll<HTMLElement>(':scope > [role^="menuitem"]')];
  }

  function entry(): HTMLElement | null {
    const root = document.getElementById(id);
    if (!root || initial === 'menu') return root;
    const index = nextIndex(-1, items.length, initial === 'first' ? 'Home' : 'End', { isDisabled });
    return (index === null ? null : itemsIn(root)[index]) ?? root;
  }

  function pick(item: MenuItem) {
    if (item.disabled) return;
    onpick(item);
  }

  function tab(event: KeyboardEvent) {
    onclose('tab');
    // On the page the browser moves on from the owner; a dialog keeps Tab inside itself.
    const dialog = topModalLayer();
    if (!dialog) return;
    event.preventDefault();
    cycleFocus(dialog, event.shiftKey);
  }

  // Capture: the menu's keys are its own before the layer's handler sees Tab.
  function onKeydown(event: KeyboardEvent) {
    if (event.isComposing || event.ctrlKey || event.metaKey || event.altKey) return;
    const root = event.currentTarget as HTMLElement;
    const elements = itemsIn(root);
    const current = elements.indexOf(document.activeElement as HTMLElement);
    if (event.key === 'Tab') {
      tab(event);
      return;
    }
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      if (current >= 0) pick(items[current]);
      return;
    }
    const next = nextIndex(current, items.length, event.key, { isDisabled });
    const found =
      next ?? (isTypeaheadKey(event) ? typeahead(event.key, items.map((item) => item.label), current, isDisabled) : null);
    if (found === null) return;
    event.preventDefault();
    elements[found]?.focus({ preventScroll: true });
  }
</script>

<div
  class="menu"
  role="menu"
  {id}
  aria-labelledby={label ? undefined : labelledby}
  aria-label={label}
  onkeydowncapture={onKeydown}
  {@attach layer({ modal: false, owner: () => owner(), returnFocus: () => owner(), initialFocus: entry, dismiss: () => onclose('dismiss') })}
  {@attach place}
>
  {#each items as item, index (item.id)}
    {#if item.separator && index > 0}
      <div class="separator" role="separator"></div>
    {/if}
    <button
      type="button"
      class="item"
      class:danger={item.danger}
      role={menuItemRole(item)}
      aria-checked={item.checked}
      aria-disabled={item.disabled || undefined}
      tabindex="-1"
      data-id={item.id}
      onclick={() => pick(item)}
      onpointermove={(event) => {
        if (!item.disabled && document.activeElement !== event.currentTarget) event.currentTarget.focus({ preventScroll: true });
      }}
    >
      {#if item.dot}<span class="dot {item.dot}" aria-hidden="true"></span>{/if}
      <span class="label">{item.label}</span>
      {#if item.checked}<Check size="1.5rem" strokeWidth={2.2} aria-hidden="true" />{/if}
    </button>
  {/each}
</div>

<style>
  .menu {
    position: fixed;
    z-index: var(--z-layer);
    min-width: 19rem;
    padding: 0.4rem;
    overflow-y: auto;
    overscroll-behavior: contain;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: popup-in var(--dur-fast) var(--ease);
  }

  /* Opened by pointer, the menu itself holds focus for the arrows; the items show where focus is. */
  .menu:focus-visible {
    outline: none;
  }

  .item {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    width: 100%;
    padding: 0.7rem 1rem;
    border-radius: var(--radius-sm);
    font-size: var(--font-sm);
    color: var(--text-2);
    text-align: left;
    white-space: nowrap;
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .item:hover,
  .item:focus {
    background: var(--hover-strong);
    color: var(--text);
  }

  .item:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: -2px;
  }

  .item[aria-disabled='true'] {
    color: var(--text-3);
    background: none;
    cursor: default;
  }

  .label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .item :global(svg) {
    flex-shrink: 0;
    color: var(--text-3);
  }

  .dot {
    width: 0.8rem;
    height: 0.8rem;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .dot.online {
    background: var(--success);
  }

  .dot.away {
    background: var(--warning);
  }

  .dot.busy {
    background: var(--danger);
  }

  .dot.offline {
    background: var(--text-3);
  }

  .item.danger:not([aria-disabled='true']) {
    color: var(--danger);
  }

  .item.danger:not([aria-disabled='true']):hover,
  .item.danger:not([aria-disabled='true']):focus {
    background: var(--danger-subtle);
  }

  .separator {
    height: 1px;
    margin: 0.4rem;
    background: var(--border);
  }
</style>
