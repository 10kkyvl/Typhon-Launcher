<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';
  import { X } from '@lucide/svelte';
  import { refuseWhile } from './refuse';
  import Spinner from './Spinner.svelte';
  import Tooltip from './Tooltip.svelte';

  /**
   * A tag that can be taken away (`onremove`) is its text and a remove button named `removeLabel`,
   * shown as that button's hint. The remove button is the keyboard path: Tab reaches it, Enter or Space
   * presses it. Backspace and Delete on a chip do nothing: a chip is not a text field, and a key that
   * deletes what the user cannot see targeted would surprise. Where focus goes after a removal is the
   * list's to decide (TagEditor), since only the list knows the neighbours. The remove button is the
   * control of a removable chip: `busy`, `disabled` and `reason` act on it, as on a pressable chip (a
   * removal in flight cannot be fired twice), and other attributes go to it. Its text does nothing, so
   * an `onclick`, a button `type` or an `end` mark is refused by type.
   *
   * Given `href` a chip is a real link (`<a>`, its `target` and `rel` passed on) with the chip's look:
   * a store page, a wiki. It goes somewhere, so it is never on or off, removable or disabled. `end` is
   * a mark after the text (the external-link sign), a bare icon sized here and hidden from the name.
   */
  type Linked =
    | { href?: undefined; target?: undefined; rel?: undefined }
    | {
        href: string;
        target?: HTMLAnchorAttributes['target'];
        rel?: HTMLAnchorAttributes['rel'];
        selected?: undefined;
        onremove?: undefined;
        removeLabel?: undefined;
        disabled?: undefined;
        reason?: undefined;
        busy?: undefined;
      };
  type Removable =
    | { onremove?: undefined; removeLabel?: undefined }
    | {
        onremove: () => void;
        removeLabel: string;
        /** Not on a removable chip: the hint would land on its remove button. */
        hint?: undefined;
        onclick?: undefined;
        type?: undefined;
        end?: undefined;
      };

  // Other attributes (id, aria-*, data-*, handlers, attachments) go to the <button>.
  let {
    selected,
    variant = 'outline',
    type = 'button',
    hint,
    href,
    target,
    rel,
    end,
    disabled = false,
    reason,
    busy = false,
    onclick,
    onremove,
    removeLabel,
    children,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'onclick' | 'children' | 'title' | 'disabled'> & {
    /**
     * Drawn in the accent. On a chip that is pressed to turn something on and off it is also its
     * state (aria-pressed); on the trigger of a menu it only shows that a choice is applied, and the
     * trigger props say whether the menu is open.
     */
    selected?: boolean;
    variant?: 'filled' | 'outline' | 'quiet';
    /** What pressing the chip does, when its text does not say it: shown as a tooltip, its description. */
    hint?: string;
    /** Out of reach (native disabled); with `reason` it stays reachable and says why, as Button does. */
    disabled?: boolean;
    /**
     * Why the chip cannot be pressed now: while disabled with a reason it stays in the Tab order
     * (aria-disabled), shows the reason as its hint and is described by it, and ignores a press,
     * Enter and Space. Pass '' while there is none, so it stays one element as the state flips.
     */
    reason?: string;
    /**
     * Its own request is running (a vote, a reaction): focus stays on it, it says it is busy and
     * unavailable, ignores presses, and draws no hover or press. The spinner stands over the label,
     * which keeps its place (the chip does not change width) and stays the chip's name.
     */
    busy?: boolean;
    onclick?: (e: MouseEvent) => void;
    /** A mark after the text (the external-link sign): a bare icon. */
    end?: Snippet;
    children?: Snippet;
  } & Removable &
    Linked = $props();

  const toggle = $derived(selected !== undefined && onclick !== undefined && !rest['aria-haspopup']);
  const refused = $derived(disabled && Boolean(reason));
  const blocked = $derived(refused || busy);
  const guard = refuseWhile(() => blocked);
  // A caller that still says aria-disabled by hand keeps it, and now gets the look with it.
  const unavailable = $derived(blocked ? 'true' : rest['aria-disabled']);
  const tip = $derived(refused ? reason : hint);
  const removable = $derived(onremove !== undefined && removeLabel !== undefined);
  // The attributes a link takes from what a button would: handlers and aria-* read the same.
  const linkRest = $derived(rest as HTMLAnchorAttributes);
</script>

{#snippet content()}
  <span class="content" class:waiting={busy}
    >{@render children?.()}{#if end}<span class="end" data-part="chip-end" aria-hidden="true">{@render end()}</span>{/if}</span
  >{#if busy}<span class="busy-mark" data-busy-mark><Spinner size="sm" tone="current" /></span>{/if}
{/snippet}

{#snippet chip()}
  {#if href !== undefined}
    <a {...linkRest} class="chip {variant}" {href} {target} {rel} {onclick}>{@render content()}</a>
  {:else if removable}
    <span class="chip removable {variant}" class:selected>
      <span class="text">{@render children?.()}</span>
      <Tooltip text={refused ? (reason ?? '') : (removeLabel ?? '')} describe={refused}>
        <button
          {...rest}
          type="button"
          class="remove"
          aria-label={removeLabel}
          aria-disabled={unavailable}
          aria-busy={busy || undefined}
          disabled={disabled && !refused}
          onclick={onremove}
          {@attach reason !== undefined || busy ? guard : undefined}
        >
          {#if busy}<Spinner size="sm" tone="current" />{:else}<X strokeWidth={2} aria-hidden="true" />{/if}
        </button>
      </Tooltip>
    </span>
  {:else}
    <button
      {...rest}
      {type}
      class="chip {variant}"
      class:selected
      aria-pressed={toggle ? selected : undefined}
      aria-disabled={unavailable}
      aria-busy={busy || undefined}
      disabled={disabled && !refused}
      {onclick}
      {@attach reason !== undefined || busy ? guard : undefined}
    >
      {@render content()}
    </button>
  {/if}
{/snippet}

{#if (hint !== undefined || reason !== undefined) && !removable}
  <Tooltip text={tip ?? ''}>{@render chip()}</Tooltip>
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

  button.chip:active:not(:disabled, [aria-disabled='true']) {
    transform: scale(0.96);
  }

  /* With animations off a press changes nothing but the colour. */
  @media (prefers-reduced-motion: reduce) {
    button.chip:active:not(:disabled, [aria-disabled='true']) {
      transform: none;
    }
  }

  :global(.no-anim) button.chip:active:not(:disabled, [aria-disabled='true']) {
    transform: none;
  }

  .outline {
    background: transparent;
    border-color: var(--border-strong);
    color: var(--text-2);
  }

  button.outline:hover:not(:disabled, [aria-disabled='true']) {
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

  button.filled:hover:not(:disabled, [aria-disabled='true']) {
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

  button.quiet:hover:not(:disabled, [aria-disabled='true']),
  .quiet.selected,
  .quiet[aria-expanded='true'] {
    background: var(--hover-strong);
    color: var(--text);
  }

  button.chip:is(:disabled, [aria-disabled='true']) {
    opacity: 0.45;
    cursor: default;
  }

  /* Busy is not unavailable: it keeps its colour while its own request runs. */
  /* The label as one item of the chip's row, its own parts spaced as the chip spaces them. */
  .content {
    display: inline-flex;
    align-items: center;
    gap: inherit;
  }

  /* Laid out but unseen while busy: the chip keeps its width and its name. */
  .content.waiting {
    opacity: 0;
  }

  .busy-mark {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
  }

  button.chip[aria-busy='true'] {
    position: relative;
    opacity: 1;
    cursor: progress;
  }

  a.chip {
    text-decoration: none;
  }

  a.outline:hover {
    background: var(--hover);
    color: var(--text);
  }

  a.filled:hover {
    background: var(--surface-4);
    color: var(--text);
  }

  a.quiet:hover {
    background: var(--hover-strong);
    color: var(--text);
  }

  .end {
    display: flex;
    flex-shrink: 0;
  }

  .end :global(svg) {
    width: var(--chip-mark);
    height: var(--chip-mark);
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

  .remove:hover:not(:disabled, [aria-disabled='true']) {
    background: var(--hover-strong);
    opacity: 1;
  }

  .remove:is(:disabled, [aria-disabled='true']) {
    opacity: 0.45;
    cursor: default;
  }

  .remove[aria-busy='true'] {
    cursor: progress;
  }
</style>
