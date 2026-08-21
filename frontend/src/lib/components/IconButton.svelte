<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    label,
    size = 'md',
    active = false,
    disabled = false,
    onclick,
    children,
  }: {
    label: string;
    size?: 'md' | 'sm';
    active?: boolean;
    disabled?: boolean;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();
</script>

<button class="icon-btn {size}" class:active aria-label={label} title={label} {disabled} {onclick}>
  {@render children?.()}
</button>

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
      color var(--dur) var(--ease);
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
