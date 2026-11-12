<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Spinner from './Spinner.svelte';
  import Tooltip from './Tooltip.svelte';

  // Other attributes (id, aria-*, data-*, handlers, attachments) go to the <button>.
  let {
    variant = 'secondary',
    size = 'md',
    type = 'button',
    disabled = false,
    pressed,
    reason,
    busy = false,
    block = false,
    icon = false,
    join,
    minWidth,
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
     * keyboard: while disabled with a reason it stays in the Tab order (aria-disabled), is described by
     * the reason, and ignores every activation: a press, Enter, Space, a form submit, and on a menu
     * button the arrows that open its menu. The click and key handlers spread onto it see none of
     * those; a capture or touch handler a caller adds is not covered. Every other key passes. A caller
     * whose reason comes and goes passes '' while there is none, not undefined: the button then stays
     * one element, and keeps focus, as the state flips.
     */
    reason?: string;
    /**
     * The button's own action is running: a spinner joins the label (which stays, so the width does
     * not jump), activations are ignored and it says it is busy. It is not disabled: focus stays on it.
     */
    busy?: boolean;
    /** Fills the width of its row. */
    block?: boolean;
    /** Holds one icon and no text: as wide as it is high. It needs an `aria-label`. */
    icon?: boolean;
    /**
     * The side on which a neighbour is attached, the two reading as one control (the play button and
     * its profile chevron): no rounding and no gap on that side.
     */
    join?: 'start' | 'end';
    /** A CSS length the button never gets narrower than, so a label that changes does not move its row. */
    minWidth?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } = $props();

  const refused = $derived(disabled && Boolean(reason));
  const blocked = $derived(refused || busy);

  // Only the keys that press a button are held back. Focus stays on a busy button for as long as its
  // action runs, so every other key (Tab, Escape, the arrows, the window's shortcuts) must pass.
  const ACTIVATION_KEYS = new Set(['Enter', ' ']);
  // A menu button also opens on these (WAI-ARIA menu button).
  const POPUP_KEYS = new Set(['ArrowDown', 'ArrowUp']);
  const ACTIVATIONS = ['click', 'pointerdown', 'pointerup', 'mousedown', 'mouseup', 'keydown', 'keyup'];

  // Capture on the button itself runs before any handler on it or above it (Svelte's delegated ones
  // included), so nothing spread onto it sees a blocked activation.
  const guard: Attachment<HTMLButtonElement> = (node) => {
    const stop = (event: Event) => {
      if (!blocked) return;
      if (event instanceof KeyboardEvent && !ACTIVATION_KEYS.has(event.key)) {
        if (!POPUP_KEYS.has(event.key) || !node.hasAttribute('aria-haspopup')) return;
      }
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
    class:block
    class:icon
    class:join-start={join === 'start'}
    class:join-end={join === 'end'}
    style:min-width={minWidth}
    disabled={disabled && !(guarded && reason)}
    aria-disabled={(guarded && refused) || busy ? 'true' : undefined}
    aria-busy={busy || undefined}
    aria-pressed={pressed}
    {onclick}
    {@attach guarded || busy ? guard : undefined}
  >
    {#if busy}<Spinner size="sm" tone="current" />{/if}{@render children?.()}
  </button>
{/snippet}

{#if reason !== undefined}
  <Tooltip text={refused ? (reason ?? '') : ''}>{@render button(true)}</Tooltip>
{:else}
  {@render button(false)}
{/if}

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    vertical-align: middle;
    gap: var(--space-2);
    height: var(--control-md);
    padding: 0 var(--space-4);
    font-size: var(--font-md);
    font-weight: 550;
    border-radius: var(--radius-control);
    border: var(--control-border-w) solid transparent;
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

  /* Busy is not unavailable: it keeps its full colour while its own action runs. */
  .btn[aria-busy='true'] {
    opacity: 1;
    cursor: progress;
  }

  .sm {
    height: var(--control-sm);
    padding: 0 var(--space-3);
    font-size: var(--font-sm);
  }

  .lg {
    height: var(--control-lg);
    padding: 0 var(--space-5);
    font-size: var(--font-lg);
  }

  .block {
    width: 100%;
  }

  .icon {
    aspect-ratio: 1;
    padding: 0;
  }

  .join-end {
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
  }

  .join-start {
    margin-left: var(--control-seam);
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
  }

  .primary {
    background: var(--accent);
    color: var(--accent-on, var(--accent-on-default));
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

  /* A tinted fill with a full border. On the tint --danger alone is under 4.5:1, so the label leans
     towards the text colour: lighter on a dark theme, darker on a light one (theme-contrast.test.ts). */
  .danger {
    background: var(--danger-subtle);
    border-color: var(--danger);
    color: color-mix(in srgb, var(--danger) 80%, var(--text));
  }

  .danger:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--danger);
    color: var(--bg);
  }

  .btn:active:not(:disabled, [aria-disabled='true']) {
    transform: scale(0.97);
    box-shadow: none;
  }

  /* The two halves of a split button read as one control: one half lifting or pressing in alone would
     step the seam between them, so a joined half stays where it is. */
  .btn:is(.join-start, .join-end):is(:hover, :active):not(:disabled, [aria-disabled='true']) {
    transform: none;
    box-shadow: none;
  }

  /* With animations off a hover or a press changes colour only. */
  @media (prefers-reduced-motion: reduce) {
    .btn:is(:hover, :active):not(:disabled, [aria-disabled='true']) {
      transform: none;
    }
  }

  :global(.no-anim) .btn:is(:hover, :active):not(:disabled, [aria-disabled='true']) {
    transform: none;
  }

  /* The in-game overlay is drawn at the density the main window had before 2.0 and keeps the controls
     of that time until it gets its own pass (the root says so with data-controls="1x"). */
  :global([data-controls='1x']) .btn {
    gap: 0.7rem;
    padding: 0 1.5rem;
    font-size: var(--font-sm);
    font-weight: 500;
    border-radius: var(--radius-md);
  }

  :global([data-controls='1x']) .sm {
    padding: 0 1.2rem;
    font-size: var(--font-xs);
  }

  :global([data-controls='1x']) .lg {
    padding: 0 2rem;
    font-size: var(--font-md);
    font-weight: 600;
  }

  :global([data-controls='1x']) .icon {
    padding: 0;
  }

  /* The 1.x border was a muted red: --danger a ninth of the way to the top surface draws it in the
     dark palette (control-tokens.test.ts) and follows any other theme. */
  :global([data-controls='1x']) .danger {
    background: transparent;
    border-color: color-mix(in srgb, color-mix(in srgb, var(--danger) 88.7%, var(--surface-4)) 30%, transparent);
    color: var(--danger);
  }

  :global([data-controls='1x']) .danger:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--danger-subtle);
    border-color: color-mix(in srgb, color-mix(in srgb, var(--danger) 88.7%, var(--surface-4)) 50%, transparent);
    color: var(--danger);
  }
</style>
