<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Tooltip from './Tooltip.svelte';

  // Other attributes (id, aria-*, data-*, handlers, attachments) go to the <button>.
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
  }: Omit<HTMLButtonAttributes, 'class' | 'onclick' | 'children' | 'title'> & {
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
    size?: 'md' | 'sm' | 'lg';
    disabled?: boolean;
    pressed?: boolean;
    /**
     * Why the button is disabled. A disabled button cannot take focus, so its reason would reach no
     * keyboard: while disabled with a reason it stays in the Tab order (aria-disabled), ignores every
     * activation (press, Enter, Space, a form submit, handlers spread onto it) and is described by the
     * reason. A caller whose reason comes and goes passes '' while there is none, not undefined: the
     * button then stays one element, and keeps focus, as the state flips.
     */
    reason?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();

  const blocked = $derived(disabled && Boolean(reason));

  // Keys that do not activate it still work: Tab moves on, Escape closes the dialog or hint around it.
  const PASSING_KEYS = new Set(['Tab', 'Escape']);
  const ACTIVATIONS = ['click', 'pointerdown', 'pointerup', 'mousedown', 'mouseup', 'keydown', 'keyup'];

  // Capture on the button itself runs before any handler on it or above it (Svelte's delegated ones
  // included), so nothing spread onto it sees a blocked activation.
  const guard: Attachment<HTMLButtonElement> = (node) => {
    const stop = (event: Event) => {
      if (!blocked) return;
      if (event instanceof KeyboardEvent && PASSING_KEYS.has(event.key)) return;
      if (event.type !== 'pointerdown' && event.type !== 'mousedown') event.preventDefault();
      event.stopImmediatePropagation();
    };
    for (const type of ACTIVATIONS) node.addEventListener(type, stop, true);
    return () => {
      for (const type of ACTIVATIONS) node.removeEventListener(type, stop, true);
    };
  };
</script>

{#snippet button(guarded: boolean)}
  <button
    {...rest}
    {type}
    class="btn {variant} {size}"
    disabled={disabled && !(guarded && reason)}
    aria-disabled={guarded && blocked ? 'true' : undefined}
    aria-pressed={pressed}
    {onclick}
    {@attach guarded ? guard : undefined}
  >
    {@render children?.()}
  </button>
{/snippet}

{#if reason !== undefined}
  <Tooltip text={blocked ? (reason ?? '') : ''}>{@render button(true)}</Tooltip>
{:else}
  {@render button(false)}
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
