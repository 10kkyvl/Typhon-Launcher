<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Tooltip from './Tooltip.svelte';

  // Other attributes (id, aria-*, data-*, handlers, attachments) go to the <button>.
  let {
    label,
    size = 'md',
    type = 'button',
    active = false,
    disabled = false,
    tooltip = true,
    onclick,
    children,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'onclick' | 'children' | 'title'> & {
    /** The button's name; shown as its tooltip too. */
    label: string;
    size?: 'md' | 'sm';
    active?: boolean;
    disabled?: boolean;
    /**
     * Off: no tooltip on the layer stack; the browser's own title shows the label instead. For the
     * overlay window, whose Escape hides the overlay and must not close a hint first.
     */
    tooltip?: boolean;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();
</script>

{#snippet button(title: string | undefined)}
  <button {...rest} {type} class="icon-btn {size}" class:active aria-label={label} {title} {disabled} {onclick}>
    {@render children?.()}
  </button>
{/snippet}

{#if tooltip}
  <Tooltip text={label} describe={false}>{@render button(undefined)}</Tooltip>
{:else}
  {@render button(label)}
{/if}

<style>
  .icon-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-sm);
    color: var(--text-2);
    flex-shrink: 0;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .icon-btn:active:not(:disabled) {
    transform: scale(0.9);
  }

  .md {
    width: 3.6rem;
    height: 3.6rem;
  }

  .sm {
    width: 3.2rem;
    height: 3.2rem;
  }

  .icon-btn:hover:not(:disabled) {
    background: var(--hover-strong);
    color: var(--text);
  }

  .icon-btn.active {
    color: var(--accent-text);
  }

  .icon-btn:disabled {
    opacity: 0.35;
    cursor: default;
  }
</style>
