<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import CountBadge from './CountBadge.svelte';
  import { countShown } from './countBadge';
  import { refuseWhile } from './refuse';
  import Spinner from './Spinner.svelte';
  import Tooltip from './Tooltip.svelte';

  type Counted =
    | { count?: undefined; countLabel?: undefined }
    | {
        /** A count on the button's corner (CountBadge): unread messages, requests. */
        count: number;
        /** The button's name while the count shows: its label with the count said ("Friends, 3 requests"). */
        countLabel: string;
      };

  // Other attributes (id, aria-*, data-*, handlers, attachments) go to the <button>.
  let {
    label,
    size = 'md',
    type = 'button',
    variant = 'ghost',
    pressed,
    lit = false,
    disabled = false,
    reason,
    busy = false,
    tooltip = true,
    hint,
    count,
    countLabel,
    onclick,
    children,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'onclick' | 'children' | 'title'> & {
    /** The button's name; shown as its tooltip too. */
    label: string;
    size?: 'md' | 'sm' | 'lg';
    /**
     * `secondary` stands on a surface of its own with a border, next to full buttons. `primary` is
     * filled in the accent as the primary `Button` is: the one main action of its place (send, the
     * chat's opener).
     */
    variant?: 'ghost' | 'secondary' | 'primary';
    /**
     * For a toggle whose name stays the same in both states: whether it is on (aria-pressed). Leave it
     * out for a plain action, and for a button that opens a menu or a panel, which says
     * `aria-expanded` through its trigger props. A button that shows or hides something below it is
     * a disclosure: it passes `aria-expanded` and `aria-controls` (the id of what it shows).
     */
    pressed?: boolean;
    /**
     * Drawn in the accent, as an "on" toggle is, and nothing more: it tells assistive technology
     * nothing. For a button whose name already says the state ("Add to favourites" / "Remove from
     * favourites"), and for one whose state another attribute tells (`aria-current` for the selected
     * item, `aria-expanded` for the trigger of what it opened).
     */
    lit?: boolean;
    disabled?: boolean;
    /**
     * Why it is disabled, as on Button: while disabled with a reason it stays in the Tab order
     * (aria-disabled), shows the reason as its hint and is described by it, and ignores every
     * activation. For a button that can turn disabled while it has focus (the history buttons): a
     * disabled button cannot hold focus, which would fall to the page. Pass '' while there is none, so
     * it stays one element as the state flips.
     */
    reason?: string;
    /**
     * The button's own action is running, as on Button: a spinner stands in the icon's place (the box
     * keeps its size), activations are ignored and it says it is busy. It is not disabled: focus and
     * the name stay on it, and its hint stays the label.
     */
    busy?: boolean;
    /**
     * Off: no tooltip on the layer stack; the browser's own title shows the label instead. For the
     * overlay window, whose Escape hides the overlay and must not close a hint first.
     */
    tooltip?: boolean;
    /**
     * What pressing it does beyond its name (the limits of an attachment): shown as its tooltip in
     * place of the label, and its description. The name stays `label`.
     */
    hint?: string;
    onclick?: (e: MouseEvent) => void;
    children?: Snippet;
  } & Counted = $props();

  const counted = $derived(count !== undefined && countShown(count) !== '');
  const name = $derived(counted && countLabel ? countLabel : label);

  const refused = $derived(disabled && Boolean(reason));
  const guard = refuseWhile(() => refused || busy);

  // The browser's own title shows the label where the tooltip cannot: the overlay window, and a
  // disabled button, which takes no focus and shows no tooltip (it cannot be announced twice).
  const nativeTitle = $derived(!tooltip || (disabled && !refused) ? label : undefined);
</script>

{#snippet button()}
  <button
    {...rest}
    {type}
    class="icon-btn {size} {variant}"
    class:lit
    class:counted
    aria-label={name}
    aria-pressed={pressed}
    aria-disabled={refused || busy ? 'true' : undefined}
    aria-busy={busy || undefined}
    title={nativeTitle}
    disabled={disabled && !refused}
    {onclick}
    {@attach reason !== undefined || busy ? guard : undefined}
  >
    {#if busy}<Spinner size="sm" tone="current" />{:else}{@render children?.()}{/if}
    {#if count !== undefined && countLabel !== undefined}
      <CountBadge {count} label={countLabel} place="corner" on={variant === 'primary' ? 'accent' : 'surface'} />
    {/if}
  </button>
{/snippet}

{#if tooltip}
  <Tooltip text={refused ? (reason ?? '') : (hint ?? label)} describe={refused || hint !== undefined}>{@render button()}</Tooltip>
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
    border: var(--control-border-w) solid transparent;
    border-radius: var(--radius-control);
    color: var(--text-2);
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease),
      color var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .icon-btn:active:not(:disabled, [aria-disabled='true']) {
    transform: scale(0.9);
  }

  /* With animations off a press changes nothing but the colour. */
  @media (prefers-reduced-motion: reduce) {
    .icon-btn:active:not(:disabled, [aria-disabled='true']) {
      transform: none;
    }
  }

  :global(.no-anim) .icon-btn:active:not(:disabled, [aria-disabled='true']) {
    transform: none;
  }

  /* The corner a count stands on. */
  .counted {
    position: relative;
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

  .icon-btn:hover:not(:disabled, [aria-disabled='true']),
  .icon-btn[aria-expanded='true'] {
    background: var(--hover-strong);
    color: var(--text);
  }

  .secondary:hover:not(:disabled, [aria-disabled='true']),
  .secondary[aria-expanded='true'] {
    background: var(--surface-4);
  }

  .icon-btn:is(.lit, [aria-pressed='true']) {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .icon-btn.primary {
    background: var(--accent);
    color: var(--accent-on, var(--accent-on-default));
  }

  .icon-btn.primary:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--accent-hover);
    color: var(--accent-on, var(--accent-on-default));
  }

  .icon-btn:disabled,
  .icon-btn[aria-disabled='true'] {
    opacity: 0.35;
    cursor: default;
  }

  /* Busy is not unavailable: it keeps its full colour while its own action runs. */
  .icon-btn[aria-busy='true'] {
    opacity: 1;
    cursor: progress;
  }

  /* The in-game overlay keeps the icon button it was drawn with (see Button.svelte): squarer, at the
     sizes of that time, an "on" state told by colour alone. */
  :global([data-controls='1x']) .icon-btn {
    width: 3.6rem;
    height: 3.6rem;
    border: 0;
    border-radius: var(--radius-sm);
  }

  :global([data-controls='1x']) .sm {
    width: 3.2rem;
    height: 3.2rem;
  }

  :global([data-controls='1x']) .icon-btn:is(.lit, [aria-pressed='true'], [aria-expanded='true']) {
    background: transparent;
  }

  :global([data-controls='1x']) .icon-btn:is(.lit, [aria-pressed='true'], [aria-expanded='true']):hover:not(:disabled) {
    background: var(--hover-strong);
  }
</style>
