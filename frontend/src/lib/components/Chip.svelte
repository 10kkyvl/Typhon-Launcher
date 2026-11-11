<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  // Other attributes (id, aria-*, data-*, handlers, attachments) go to the <button>.
  let {
    selected = false,
    variant = 'outline',
    type = 'button',
    title,
    onclick,
    children,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'onclick' | 'children'> & {
    selected?: boolean;
    variant?: 'filled' | 'outline' | 'quiet';
    title?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();
</script>

<button {...rest} {type} class="chip {variant}" class:selected {title} {onclick}>
  {@render children?.()}
</button>

<style>
  .chip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.6rem;
    height: var(--control-sm);
    padding: 0 calc(1.2rem - 1px);
    border-radius: var(--radius-xl);
    border: 2px solid transparent;
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease),
      color var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .chip:active {
    transform: scale(0.96);
  }

  .outline {
    background: transparent;
    border-color: var(--border);
    color: var(--text-2);
  }

  .outline:hover {
    border-color: var(--border-strong);
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

  .filled:hover {
    background: var(--surface-4);
    color: var(--text);
  }

  .filled.selected,
  .filled[aria-expanded='true'] {
    background: var(--accent);
    color: var(--accent-on, #fff);
  }

  .quiet {
    background: transparent;
    color: var(--text-3);
  }

  .quiet:hover,
  .quiet.selected,
  .quiet[aria-expanded='true'] {
    background: var(--hover);
    color: var(--text);
  }
</style>
