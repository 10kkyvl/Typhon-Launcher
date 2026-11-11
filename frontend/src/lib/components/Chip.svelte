<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import { X } from '@lucide/svelte';
  import Tooltip from './Tooltip.svelte';

  /**
   * A tag that can be taken away (`onremove`) is its text and a remove button named `removeLabel`,
   * shown as that button's hint. The remove button is the keyboard path: Tab reaches it, Enter or Space
   * presses it. Backspace and Delete on a chip do nothing: a chip is not a text field, and a key that
   * deletes what the user cannot see targeted would surprise. Where focus goes after a removal is the
   * list's to decide (TagEditor), since only the list knows the neighbours.
   */
  type Removable =
    | { onremove?: undefined; removeLabel?: undefined }
    | {
        onremove: () => void;
        removeLabel: string;
        /** Not on a removable chip: the hint would land on its remove button. */
        hint?: undefined;
      };

  // Other attributes (id, aria-*, data-*, handlers, attachments) go to the <button>.
  let {
    selected,
    variant = 'outline',
    type = 'button',
    hint,
    onclick,
    onremove,
    removeLabel,
    children,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'onclick' | 'children' | 'title'> & {
    /**
     * Drawn in the accent. On a chip that is pressed to turn something on and off it is also its
     * state (aria-pressed); on the trigger of a menu it only shows that a choice is applied, and the
     * trigger props say whether the menu is open.
     */
    selected?: boolean;
    variant?: 'filled' | 'outline' | 'quiet';
    /** What pressing the chip does, when its text does not say it: shown as a tooltip, its description. */
    hint?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } & Removable = $props();

  const toggle = $derived(selected !== undefined && onclick !== undefined && !rest['aria-haspopup']);
</script>

{#snippet chip()}
  {#if onremove !== undefined && removeLabel !== undefined}
    <span class="chip removable {variant}" class:selected>
      <span class="text">{@render children?.()}</span>
      <Tooltip text={removeLabel} describe={false}>
        <button type="button" class="remove" aria-label={removeLabel} onclick={onremove}>
          <X strokeWidth={2} aria-hidden="true" />
        </button>
      </Tooltip>
    </span>
  {:else}
    <button {...rest} {type} class="chip {variant}" class:selected aria-pressed={toggle ? selected : undefined} {onclick}>
      {@render children?.()}
    </button>
  {/if}
{/snippet}

{#if hint}
  <Tooltip text={hint}>{@render chip()}</Tooltip>
{:else}
  {@render chip()}
{/if}

<style>
  .chip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-1);
    height: var(--control-sm);
    padding: 0 var(--space-3);
    border-radius: var(--radius-sm);
    border: var(--control-border-w) solid transparent;
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease),
      color var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  button.chip:active {
    transform: scale(0.96);
  }

  /* With animations off a press changes nothing but the colour. */
  @media (prefers-reduced-motion: reduce) {
    button.chip:active {
      transform: none;
    }
  }

  :global(.no-anim) button.chip:active {
    transform: none;
  }

  .outline {
    background: transparent;
    border-color: var(--border-strong);
    color: var(--text-2);
  }

  button.outline:hover {
    background: var(--hover);
    color: var(--text);
  }

  .outline.selected,
  .outline[aria-expanded='true'] {
    border-color: var(--accent-bright);
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .filled {
    background: var(--surface-3);
    color: var(--text-2);
  }

  button.filled:hover {
    background: var(--surface-4);
    color: var(--text);
  }

  .filled.selected,
  .filled[aria-expanded='true'] {
    background: var(--accent);
    color: var(--accent-on, var(--accent-on-default));
  }

  .quiet {
    background: transparent;
    color: var(--text-2);
  }

  button.quiet:hover,
  .quiet.selected,
  .quiet[aria-expanded='true'] {
    background: var(--hover-strong);
    color: var(--text);
  }

  .removable {
    padding-right: 0;
  }

  .remove {
    display: grid;
    place-items: center;
    align-self: stretch;
    width: var(--chip-remove-w);
    border-radius: inherit;
    color: inherit;
    opacity: 0.8;
    transition:
      background var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }

  .remove :global(svg) {
    width: var(--chip-remove-icon);
    height: var(--chip-remove-icon);
  }

  .remove:hover {
    background: var(--hover-strong);
    opacity: 1;
  }
</style>
