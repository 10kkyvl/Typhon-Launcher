<script lang="ts">
  import { flushSync, untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { ChevronDown, Check } from '@lucide/svelte';
  import { layer } from '../layers';
  import { anchorTo, outOfSight } from '../utils/position';
  import { createTypeahead, isTypeaheadKey, nextIndex } from '../utils/roving';
  import type { SelectOption } from './select';

  /**
   * Select-only combobox (WAI-ARIA APG). Focus stays on the field the whole time; the option the keys
   * are on is conveyed with `aria-activedescendant`. The list is a non-modal layer owned by the field
   * (see lib/layers): it opens out of the page under the field, at least as wide as it, so a dialog's
   * scrolling body does not cut it; the stack answers Escape, presses outside and a dialog opening.
   *
   * Two ways to hold the value:
   * - `bind:value`: choosing writes the bound value;
   * - `value` + `onchange`: the caller holds it. The field shows `value` and nothing else, so a change
   *   the caller refuses or fails to save leaves the field on what the caller still holds. With
   *   `onchange` the field never writes `value`; a caller that binds as well updates it in `onchange`.
   *
   * Choosing the option already chosen reports it again ("Other folder…" acts each time). A value no
   * option has shows as it is; an empty one shows `placeholder`.
   */
  let {
    options,
    value = $bindable(),
    width = '22rem',
    id,
    label,
    labelledby,
    placeholder,
    disabled = false,
    onchange,
  }: {
    options: SelectOption[];
    value: string;
    width?: string;
    /** Id of the field: a `<label for>` with it names the field and its list. */
    id?: string;
    /** Accessible name when no visible label names the field. */
    label?: string;
    /** Id of the visible label that names the field. */
    labelledby?: string;
    /** Shown when the value is empty and no option has it. */
    placeholder?: string;
    disabled?: boolean;
    onchange?: (id: string) => void;
  } = $props();

  const uid = $props.id();
  const fieldId = $derived(id ?? `${uid}-field`);
  const listId = `${uid}-list`;
  const optionId = (index: number) => `${uid}-option-${index}`;

  let field = $state<HTMLButtonElement>();
  let open = $state(false);
  // By option id, not index: options re-created while the list is open keep the active one.
  let activeId = $state<string | null>(null);

  const typeahead = createTypeahead();
  const isDisabled = (index: number) => Boolean(options[index]?.disabled);
  const labels = $derived(options.map((option) => option.label));
  const selectedIndex = $derived(options.findIndex((option) => option.id === value));
  const active = $derived(activeId === null ? -1 : options.findIndex((option) => option.id === activeId));
  const shown = $derived(selectedIndex >= 0 ? options[selectedIndex].label : value || placeholder || '');

  function show(index: number | null) {
    if (disabled || options.length === 0) return;
    // A field scrolled out of sight can still hold focus; its list would close at once as hidden.
    if (field && outOfSight(field)) field.scrollIntoView({ block: 'nearest' });
    const start = index ?? (selectedIndex >= 0 ? selectedIndex : nextIndex(-1, options.length, 'Home', { isDisabled }));
    activeId = start === null ? null : options[start].id;
    open = true;
  }

  // From the stack (Escape, a press outside, a dialog opening) and anchorTo: inside effects, no flush.
  function hide() {
    open = false;
    activeId = null;
  }

  // Unmounts the list now. Only from event handlers.
  function close() {
    if (!open) return;
    hide();
    flushSync();
  }

  function choose(index: number) {
    const option = options[index];
    if (!option || option.disabled) return;
    close();
    field?.focus({ preventScroll: true });
    if (onchange) onchange(option.id);
    else value = option.id;
  }

  // Enter, Space, Tab, Alt+ArrowUp: the active option, or just close when there is none to choose.
  function commit() {
    if (active >= 0 && !isDisabled(active)) choose(active);
    else close();
  }

  function move(index: number | null) {
    const option = index === null ? undefined : options[index];
    if (option) activeId = option.id;
  }

  // Ten options on, or the last enabled one short of that; null when none lies between.
  function page(step: number): number | null {
    if (options.length === 0) return null;
    const back = -Math.sign(step);
    for (let index = Math.min(Math.max(active + step, 0), options.length - 1); index !== active && index >= 0 && index < options.length; index += back) {
      if (!isDisabled(index)) return index;
    }
    return null;
  }
  function onClosedKeydown(event: KeyboardEvent) {
    const { key } = event;
    if (event.altKey ? key === 'ArrowDown' : key === 'ArrowDown' || key === 'ArrowUp' || key === 'Enter' || key === ' ') {
      event.preventDefault();
      // A held key that just chose an option must not open the list again.
      if (!event.repeat) show(null);
      return;
    }
    if (key === 'Home' || key === 'End') {
      event.preventDefault();
      show(nextIndex(-1, options.length, key, { isDisabled }));
      return;
    }
    if (isTypeaheadKey(event)) {
      event.preventDefault();
      show(typeahead(key, labels, selectedIndex, isDisabled));
    }
  }

  function onOpenKeydown(event: KeyboardEvent) {
    const { key } = event;
    if (key === 'Tab') {
      // Not prevented: focus moves on from the field, inside the dialog the field sits in when there is one.
      if (!event.repeat) commit();
      return;
    }
    if (event.altKey) {
      if (key !== 'ArrowUp') return;
      event.preventDefault();
      if (!event.repeat) commit();
      return;
    }
    if (key === 'Enter' || key === ' ') {
      event.preventDefault();
      // A held key chooses once. Space always chooses, also right after a typed character: labels such
      // as "1 MB/s" would otherwise swallow it as part of the typed word.
      if (!event.repeat) commit();
      return;
    }
    if (key === 'PageDown' || key === 'PageUp') {
      event.preventDefault();
      move(page(key === 'PageDown' ? 10 : -10));
      return;
    }
    const next = nextIndex(active, options.length, key, { isDisabled, loop: false });
    if (next !== null) {
      event.preventDefault();
      move(next);
      return;
    }
    if (isTypeaheadKey(event)) {
      event.preventDefault();
      move(typeahead(key, labels, active, isDisabled));
    }
  }

  // Capture: a delegated handler would run after the dialog's Tab trap has already moved focus on.
  function onKeydown(event: KeyboardEvent) {
    if (event.isComposing || event.ctrlKey || event.metaKey) return;
    if (open) onOpenKeydown(event);
    else onClosedKeydown(event);
  }

  // The text of the `<label for>` elements naming the field. The list is not labelled by the field
  // itself: a browser reads a combobox used as a label as its chosen option.
  function forLabels(): string | undefined {
    const text = [...(field?.labels ?? [])].map((element) => element.textContent?.trim() ?? '').filter(Boolean).join(' ');
    return text || undefined;
  }

  // The list leaves with its field: it closes when the field scrolls out of sight.
  const place: Attachment<HTMLElement> = (node) =>
    untrack(() => anchorTo(node, () => field, { placement: 'bottom-start', offset: 4, matchWidth: true, onhide: hide }));

  // Scrolls the list, and only the list, to the active option. Runs again when the active option changes.
  const reveal: Attachment<HTMLElement> = (node) => {
    const index = active;
    untrack(() => {
      const option = node.querySelectorAll<HTMLElement>(':scope > [role="option"]')[index];
      if (!option) return;
      const top = option.offsetTop;
      const bottom = top + option.offsetHeight;
      if (top < node.scrollTop) node.scrollTop = top;
      else if (bottom > node.scrollTop + node.clientHeight) node.scrollTop = bottom - node.clientHeight;
    });
  };
</script>

<div class="select" style:width>
  <button
    bind:this={field}
    type="button"
    class="trigger"
    class:open
    id={fieldId}
    role="combobox"
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-controls={open ? listId : undefined}
    aria-activedescendant={open && active >= 0 ? optionId(active) : undefined}
    aria-label={label}
    aria-labelledby={label ? undefined : labelledby}
    {disabled}
    onclick={() => (open ? close() : show(null))}
    onkeydowncapture={onKeydown}
    onkeyup={(event) => {
      // Keys act on keydown; the browser's Space-keyup click on the field would toggle the list again.
      if (event.key === ' ') event.preventDefault();
    }}
  >
    <span class="value" class:placeholder={selectedIndex < 0 && !value}>{shown}</span>
    <ChevronDown size="1.5rem" strokeWidth={1.8} aria-hidden="true" />
  </button>
</div>

{#if open}
  <!-- The options take no focus: a press on the list keeps it on the field. -->
  <div
    class="listbox"
    role="listbox"
    data-part="listbox"
    id={listId}
    aria-label={label ?? (labelledby ? undefined : forLabels())}
    aria-labelledby={label ? undefined : labelledby}
    tabindex="-1"
    onmousedown={(event) => event.preventDefault()}
    {@attach layer({ modal: false, owner: () => field, dismiss: hide })}
    {@attach place}
    {@attach reveal}
  >
    {#each options as option, index (option.id)}
      <!-- svelte-ignore a11y_click_events_have_key_events (the keys are the field's, see onKeydown) -->
      <div
        class="option"
        class:active={index === active}
        role="option"
        id={optionId(index)}
        aria-selected={option.id === value}
        aria-disabled={option.disabled || undefined}
        tabindex="-1"
        onclick={() => choose(index)}
        onpointermove={() => {
          if (!option.disabled && index !== active) activeId = option.id;
        }}
      >
        <span class="label">{option.label}</span>
        {#if option.id === value}<Check size="1.4rem" strokeWidth={2} aria-hidden="true" />{/if}
      </div>
    {/each}
  </div>
{/if}

<style>
  .trigger {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    width: 100%;
    height: var(--control-md);
    padding: 0 1rem 0 1.2rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    color: var(--text);
    text-align: left;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .value {
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .value.placeholder {
    color: var(--text-3);
  }

  .trigger :global(svg) {
    color: var(--text-3);
    flex-shrink: 0;
    transition: transform var(--dur) var(--ease);
  }

  .trigger.open,
  .trigger:hover:not(:disabled) {
    border-color: var(--border-strong);
    background: var(--surface-3);
  }

  .trigger:disabled {
    opacity: 0.55;
  }

  .trigger.open :global(svg) {
    transform: rotate(180deg);
  }

  .listbox {
    z-index: var(--z-layer);
    max-height: min(36rem, 60vh);
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 0.4rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: popup-in var(--dur-fast) var(--ease);
  }

  .listbox:focus-visible {
    outline: none;
  }

  .option {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    padding: 0.7rem 0.9rem;
    border-radius: var(--radius-sm);
    font-size: var(--font-sm);
    color: var(--text-2);
    cursor: default;
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .option.active {
    background: var(--hover-strong);
    color: var(--text);
  }

  .option[aria-selected='true'] {
    color: var(--text);
  }

  .option[aria-disabled='true'] {
    color: var(--text-3);
    background: none;
  }

  .label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .option :global(svg) {
    flex-shrink: 0;
    color: var(--accent-text);
  }
</style>
