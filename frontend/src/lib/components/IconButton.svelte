<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    label,
    size = 'md',
    active = false,
    onclick,
    children,
  }: {
    label: string;
    size?: 'md' | 'sm';
    active?: boolean;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();
</script>

<button class="icon-btn {size}" class:active aria-label={label} title={label} {onclick}>
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
    width: 36px;
    height: 36px;
  }

  .sm {
    width: 30px;
    height: 30px;
  }

  .icon-btn:hover {
    background: rgba(255, 255, 255, 0.06);
    color: var(--text);
  }

  .icon-btn.active {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }
</style>
