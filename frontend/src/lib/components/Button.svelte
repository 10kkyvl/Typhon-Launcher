<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    variant = 'secondary',
    size = 'md',
    disabled = false,
    onclick,
    children,
  }: {
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
    size?: 'md' | 'sm' | 'lg';
    disabled?: boolean;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();
</script>

<button class="btn {variant} {size}" {disabled} {onclick}>
  {@render children?.()}
</button>

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.8rem;
    font-size: var(--font-md);
    font-weight: 500;
    border-radius: var(--radius-md);
    border: 1px solid transparent;
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease),
      color var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .btn:active:not(:disabled) {
    transform: translateY(1px);
  }

  .btn:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .md {
    height: var(--control-md);
    padding: 0 1.8rem;
  }

  .sm {
    height: var(--control-sm);
    padding: 0 1.4rem;
    font-size: var(--font-sm);
  }

  .lg {
    height: var(--control-lg);
    padding: 0 2.2rem;
    font-size: 1.6rem;
    font-weight: 600;
  }

  .primary {
    background: var(--accent);
    color: #fff;
  }

  .primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .secondary {
    background: var(--surface-3);
    border-color: var(--border);
    color: var(--text);
  }

  .secondary:hover:not(:disabled) {
    background: #1e2835;
    border-color: var(--border-strong);
  }

  .ghost {
    background: transparent;
    color: var(--text-2);
  }

  .ghost:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.05);
    color: var(--text);
  }

  .danger {
    background: transparent;
    border-color: rgba(217, 105, 105, 0.35);
    color: var(--danger);
  }

  .danger:hover:not(:disabled) {
    background: var(--danger-subtle);
    border-color: rgba(217, 105, 105, 0.55);
  }
</style>
