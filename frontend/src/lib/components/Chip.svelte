<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    selected = false,
    variant = 'outline',
    onclick,
    children,
  }: {
    selected?: boolean;
    variant?: 'filled' | 'outline' | 'quiet';
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();
</script>

<button type="button" class="chip {variant}" class:selected {onclick}>
  {@render children?.()}
</button>

<style>
  .chip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.6rem;
    height: var(--control-sm);
    padding: 0 1.2rem;
    border-radius: var(--radius-xl);
    border: 1px solid transparent;
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease),
      color var(--dur) var(--ease);
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

  .outline.selected {
    border-color: var(--accent);
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

  .filled.selected {
    background: var(--accent);
    color: #fff;
  }

  .quiet {
    background: transparent;
    color: var(--text-3);
  }

  .quiet:hover,
  .quiet.selected {
    background: var(--hover);
    color: var(--text);
  }
</style>
