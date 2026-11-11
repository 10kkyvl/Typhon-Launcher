<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Tooltip from './Tooltip.svelte';

  // Other attributes (id, aria-*, data-*, title, handlers, attachments) go to the <button>.
  let {
    variant = 'secondary',
    size = 'md',
    type = 'button',
    disabled = false,
    pressed,
    reason,
    onclick,
    children,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'onclick' | 'children'> & {
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
    size?: 'md' | 'sm' | 'lg';
    disabled?: boolean;
    pressed?: boolean;
    /**
     * Why the button is disabled. A disabled button cannot take focus, so its reason would reach no
     * keyboard: with a reason it stays in the Tab order (aria-disabled), ignores presses and shows the
     * reason as its description.
     */
    reason?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();
</script>

{#if disabled && reason}
  <Tooltip text={reason}>
    <button {...rest} {type} class="btn {variant} {size}" aria-disabled="true" aria-pressed={pressed}>
      {@render children?.()}
    </button>
  </Tooltip>
{:else}
  <button {...rest} {type} class="btn {variant} {size}" {disabled} aria-pressed={pressed} {onclick}>
    {@render children?.()}
  </button>
{/if}

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    vertical-align: middle;
    gap: 0.7rem;
    font-size: var(--font-sm);
    font-weight: 500;
    border-radius: var(--radius-md);
    border: 1px solid transparent;
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease),
      color var(--dur) var(--ease),
      box-shadow var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .btn:disabled,
  .btn[aria-disabled='true'] {
    opacity: 0.45;
    cursor: default;
  }

  .md {
    height: var(--control-md);
    padding: 0 1.5rem;
  }

  .sm {
    height: var(--control-sm);
    padding: 0 1.2rem;
    font-size: var(--font-xs);
  }

  .lg {
    height: var(--control-lg);
    padding: 0 2rem;
    font-size: var(--font-md);
    font-weight: 600;
  }

  .primary {
    background: var(--accent);
    color: var(--accent-on, #fff);
    border-radius: var(--radius-md);
  }

  .primary:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--accent-hover);
    box-shadow: 0 0.8rem 2rem -0.8rem var(--accent-ring);
    transform: translateY(-1px);
  }

  .secondary {
    background: var(--surface-3);
    border-color: var(--border-strong);
    color: var(--text);
  }

  .secondary:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--surface-4);
    border-color: color-mix(in srgb, var(--text) 22%, transparent);
  }

  .ghost {
    background: transparent;
    color: var(--text-2);
  }

  .ghost:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--hover-strong);
    color: var(--text);
  }

  .danger {
    background: transparent;
    border-color: rgba(217, 105, 105, 0.3);
    color: var(--danger);
  }

  .danger:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--danger-subtle);
    border-color: rgba(217, 105, 105, 0.5);
  }

  .btn:active:not(:disabled, [aria-disabled='true']) {
    transform: scale(0.97);
    box-shadow: none;
  }
</style>
