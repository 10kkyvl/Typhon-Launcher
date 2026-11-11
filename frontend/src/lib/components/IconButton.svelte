<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Tooltip from './Tooltip.svelte';

  // Other attributes (id, aria-*, data-*, handlers, attachments) go to the <button>.
  let {
    label,
    size = 'md',
    type = 'button',
    variant = 'ghost',
    pressed,
    disabled = false,
    tooltip = true,
    onclick,
    children,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'onclick' | 'children' | 'title'> & {
    /** The button's name; shown as its tooltip too. */
    label: string;
    size?: 'md' | 'sm' | 'lg';
    /** `secondary` stands on a surface of its own with a border, next to full buttons. */
    variant?: 'ghost' | 'secondary';
    /**
     * For a toggle (favourite, watch): whether it is on. Leave it out for a plain action, and for a
     * button that opens a menu or a panel, which says `aria-expanded` through its trigger props.
     */
    pressed?: boolean;
    disabled?: boolean;
    /**
     * Off: no tooltip on the layer stack; the browser's own title shows the label instead. For the
     * overlay window, whose Escape hides the overlay and must not close a hint first.
     */
    tooltip?: boolean;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();

  // The browser's own title shows the label where the tooltip cannot: the overlay window, and a
  // disabled button, which takes no focus and shows no tooltip (it cannot be announced twice).
  const nativeTitle = $derived(!tooltip || disabled ? label : undefined);
</script>

{#snippet button()}
  <button {...rest} {type} class="icon-btn {size} {variant}" aria-label={label} aria-pressed={pressed} title={nativeTitle} {disabled} {onclick}>
    {@render children?.()}
  </button>
{/snippet}

{#if tooltip}
  <Tooltip text={label} describe={false}>{@render button()}</Tooltip>
{:else}
  {@render button()}
{/if}

<style>
  .icon-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: var(--control-md);
    height: var(--control-md);
    border: 1px solid transparent;
    border-radius: var(--radius-control);
    color: var(--text-2);
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease),
      color var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .icon-btn:active:not(:disabled) {
    transform: scale(0.9);
  }

  .sm {
    width: var(--control-sm);
    height: var(--control-sm);
  }

  .lg {
    width: var(--control-lg);
    height: var(--control-lg);
  }

  .secondary {
    background: var(--surface-3);
    border-color: var(--border-strong);
    color: var(--text);
  }

  .icon-btn:hover:not(:disabled),
  .icon-btn[aria-expanded='true'] {
    background: var(--hover-strong);
    color: var(--text);
  }

  .secondary:hover:not(:disabled),
  .secondary[aria-expanded='true'] {
    background: var(--surface-4);
  }

  .icon-btn[aria-pressed='true'] {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .icon-btn:disabled {
    opacity: 0.35;
    cursor: default;
  }

  /* The in-game overlay keeps the icon button it was drawn with (see Button.svelte): squarer, at the
     sizes of that time, an "on" state told by colour alone. */
  :global([data-controls='1x']) .icon-btn {
    width: 3.6rem;
    height: 3.6rem;
    border-radius: var(--radius-sm);
  }

  :global([data-controls='1x']) .sm {
    width: 3.2rem;
    height: 3.2rem;
  }

  :global([data-controls='1x']) .icon-btn[aria-pressed='true'] {
    background: transparent;
    color: var(--accent-text);
  }

  :global([data-controls='1x']) .icon-btn[aria-pressed='true']:hover:not(:disabled) {
    background: var(--hover-strong);
  }
</style>
