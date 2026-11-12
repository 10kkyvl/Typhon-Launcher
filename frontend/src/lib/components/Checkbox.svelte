<script lang="ts">
  import { Check, Minus } from '@lucide/svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Tooltip from './Tooltip.svelte';

  /**
   * A checkbox row (WAI-ARIA checkbox): the box, the label that names it, an optional description
   * under the label and an optional short value at the row's end (a file's size), both its
   * description. Checked, unchecked or `mixed` (some of what it stands for is on); a press or Space
   * asks for checked when it is not, unchecked when it is. Enter does nothing, as on a native checkbox.
   *
   * Who holds the state: with `onchange` the caller does, and the box only asks (`onchange(next)`)
   * and shows what the caller passes back in `checked`, so a refused or failed change is never shown
   * as done; without it, `bind:checked` is written. Other attributes (id, aria-*, data-*) go to the
   * control.
   */
  let {
    checked = $bindable(false),
    label,
    description,
    aside,
    disabled = false,
    reason,
    onchange,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'role' | 'type' | 'onclick' | 'onkeydown' | 'onchange' | 'children' | 'title' | 'disabled'> & {
    checked?: boolean | 'mixed';
    label: string;
    description?: string;
    /** A short value drawn at the end of the row (a size, a count). */
    aside?: string;
    disabled?: boolean;
    /**
     * Why it is disabled, as on Button: while disabled with a reason it stays in the Tab order
     * (aria-disabled), is described by the reason, shown as its hint, and ignores a press and Space.
     * Pass '' while there is none, so it stays one element as the state flips.
     */
    reason?: string;
    onchange?: (checked: boolean) => void;
  } = $props();

  const uid = $props.id();
  const labelId = `${uid}-label`;
  const descriptionId = `${uid}-description`;
  const asideId = `${uid}-aside`;
  const described = $derived([description ? descriptionId : '', aside ? asideId : ''].filter(Boolean).join(' ') || undefined);
  const refused = $derived(disabled && Boolean(reason));

  function toggle() {
    if (disabled) return;
    const next = checked !== true;
    if (onchange) onchange(next);
    else checked = next;
  }

  // A button presses on Enter; a checkbox does not.
  function onkeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') event.preventDefault();
  }
</script>

{#snippet row()}
  <button
    {...rest}
    type="button"
    role="checkbox"
    class="checkbox"
    aria-checked={checked === 'mixed' ? 'mixed' : checked}
    aria-labelledby={labelId}
    aria-describedby={described}
    aria-disabled={refused ? 'true' : undefined}
    disabled={disabled && !refused}
    onclick={toggle}
    {onkeydown}
  >
    <span class="box" class:on={checked !== false} aria-hidden="true">
      {#if checked === 'mixed'}<Minus strokeWidth={2.6} />{:else if checked}<Check strokeWidth={2.6} />{/if}
    </span>
    <span class="text">
      <span class="label" id={labelId}>{label}</span>
      {#if description}<span class="description" id={descriptionId}>{description}</span>{/if}
    </span>
    {#if aside}<span class="aside" id={asideId}>{aside}</span>{/if}
  </button>
{/snippet}

{#if reason !== undefined}
  <Tooltip text={refused ? (reason ?? '') : ''}>{@render row()}</Tooltip>
{:else}
  {@render row()}
{/if}

<style>
  .checkbox {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    width: 100%;
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .checkbox:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--hover);
  }

  .checkbox:disabled,
  .checkbox[aria-disabled='true'] {
    opacity: 0.45;
    cursor: default;
  }

  .box {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: var(--checkbox-box);
    height: var(--checkbox-box);
    margin-top: var(--checkbox-nudge);
    border: var(--control-border-w) solid var(--border-strong);
    border-radius: var(--radius-xs);
    background: var(--surface-2);
    color: var(--accent-on, var(--accent-on-default));
    transition:
      background var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease);
  }

  .box :global(svg) {
    width: var(--checkbox-mark);
    height: var(--checkbox-mark);
  }

  .box.on {
    background: var(--accent);
    border-color: var(--accent-bright);
  }

  .text {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .label {
    font-size: var(--font-sm);
    color: var(--text);
    overflow-wrap: anywhere;
  }

  .description {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .aside {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }
</style>
