<script lang="ts" generics="T extends OptionItem">
  import type { Snippet } from 'svelte';
  import { focusQuietly } from '../layers/focus';
  import { nextIndex } from '../utils/roving';
  import type { OptionItem } from './optionList';

  /**
   * One choice from a short list of rows (WAI-ARIA radio group): a save folder among those found, an
   * executable, a match for a game. Named once: `label`, or `labelledby`, the id of the words above it.
   * Tab reaches one row, the chosen one or the first that can be chosen; the arrows move and choose,
   * wrapping at the ends; Home and End jump. A row with a `disabledReason` shows it and is described by
   * it; the arrows pass over it and a press does nothing (lib/utils/roving.ts).
   *
   * Two ways to hold the choice, as Select: `bind:value`, which choosing writes; or `value` and
   * `onchange`, where the caller holds it and the list never writes it, so a choice the caller refuses
   * does not stay on screen. A value no row has chooses none.
   *
   * A row draws its `title`, its `note` under it and its `fact` at its end; `row` draws the row's
   * content itself (a cover and a year), the list keeping the mark and the reason. A row is named by
   * its content and described by the rest. The list grows with its rows: a long one scrolls with the
   * dialog body it sits in, and a row chosen by the keys is brought into view. With no rows it draws
   * nothing: the caller says why the list is empty. When the rows change under the focus (a row gone),
   * focus stays in the list, on its Tab stop.
   *
   * `disabled` holds the whole list while the caller's write runs: the group and every row say so,
   * and no key or press chooses or moves focus. No row is natively disabled, so a focused row keeps
   * its focus and the Tab stop stays; the caller says why the list is held.
   */
  type Named = { label: string; labelledby?: undefined } | { labelledby: string; label?: undefined };

  let {
    options,
    value = $bindable(),
    label,
    labelledby,
    describedby,
    onchange,
    disabled = false,
    row,
  }: Named & {
    options: T[];
    value?: string;
    /** Id(s) of the page's text that says what the choice is for. */
    describedby?: string;
    onchange?: (id: string) => void;
    /** The list is held while the caller writes the choice; the caller says why. */
    disabled?: boolean;
    row?: Snippet<[T]>;
  } = $props();

  const uid = $props.id();
  let root = $state<HTMLElement>();

  const off = (index: number) => Boolean(options[index]?.disabledReason);
  const chosen = $derived(options.findIndex((option) => option.id === value));
  // The one Tab stop. With every row refused it is the first, so a keyboard still reaches the reasons.
  const stop = $derived.by(() => {
    if (chosen >= 0 && !off(chosen)) return chosen;
    return Math.max(0, options.findIndex((_, index) => !off(index)));
  });

  const radios = () => [...(root?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];
  const partId = (index: number, part: 'name' | 'note' | 'fact' | 'reason') => `${uid}-${index}-${part}`;

  function described(option: T, index: number): string | undefined {
    const parts = row ? [] : [option.note && partId(index, 'note'), option.fact && partId(index, 'fact')];
    return [...parts, option.disabledReason && partId(index, 'reason')].filter(Boolean).join(' ') || undefined;
  }

  function choose(index: number) {
    const option = options[index];
    if (disabled || !option || option.disabledReason || option.id === value) return;
    if (onchange) onchange(option.id);
    else value = option.id;
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const all = radios();
    const next = nextIndex(all.indexOf(event.currentTarget as HTMLElement), options.length, event.key, { orientation: 'both', isDisabled: off });
    if (next === null) return;
    event.preventDefault();
    if (disabled) return;
    choose(next);
    const target = all[next];
    if (!target) return;
    // Focus moved by script does not scroll: the dialog body brings the row in by the least movement.
    focusQuietly(target);
    if (typeof target.scrollIntoView === 'function') target.scrollIntoView({ block: 'nearest' });
  }

  // Whether focus was in the list before the rows changed: a focused row that leaves takes focus with
  // it to <body>, and the list then gives it to its Tab stop.
  let held = false;
  $effect.pre(() => {
    void options;
    held = !!root?.contains(document.activeElement);
  });
  $effect(() => {
    void options;
    if (!held) return;
    held = false;
    if (root?.contains(document.activeElement)) return;
    const target = radios()[stop];
    if (target) focusQuietly(target);
  });
</script>

{#if options.length > 0}
  <div
    class="option-list"
    role="radiogroup"
    aria-label={label}
    aria-labelledby={labelledby}
    aria-describedby={describedby}
    aria-disabled={disabled ? 'true' : undefined}
    bind:this={root}
  >
    {#each options as option, index (option.id)}
      {@const checked = option.id === value}
      <button
        type="button"
        class="option"
        role="radio"
        aria-checked={checked}
        aria-disabled={disabled || option.disabledReason ? 'true' : undefined}
        aria-labelledby={partId(index, 'name')}
        aria-describedby={described(option, index)}
        tabindex={index === stop ? 0 : -1}
        onclick={() => choose(index)}
        onmousedown={(event) => {
          if (disabled) event.preventDefault();
        }}
        {onkeydown}
      >
        <span class="mark" class:on={checked} aria-hidden="true"></span>
        <span class="body">
          {#if row}
            <span class="content" id={partId(index, 'name')}>{@render row(option)}</span>
          {:else}
            <span class="title" id={partId(index, 'name')}>{option.title}</span>
            {#if option.note}<span class="note" id={partId(index, 'note')}>{option.note}</span>{/if}
          {/if}
          {#if option.disabledReason}<span class="reason" id={partId(index, 'reason')}>{option.disabledReason}</span>{/if}
        </span>
        {#if !row && option.fact}<span class="fact" id={partId(index, 'fact')}>{option.fact}</span>{/if}
      </button>
    {/each}
  </div>
{/if}

<style>
  .option-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .option {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    min-width: 0;
    padding: 0.9rem 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text-2);
    text-align: start;
    transition:
      background var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .option:hover:not([aria-disabled='true']) {
    background: var(--hover);
    color: var(--text);
  }

  .option[aria-checked='true'] {
    border-color: var(--accent-bright);
    background: var(--hover-strong);
    color: var(--text);
  }

  .option[aria-disabled='true'] {
    color: var(--text-3);
    cursor: default;
  }

  /* Inside a scrolling dialog body: the shared ring is drawn inward, where the body does not clip it. */
  .option:focus-visible {
    outline-offset: -2px;
  }

  .mark {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    width: 1.6rem;
    height: 1.6rem;
    border: 1px solid var(--border-strong);
    border-radius: 50%;
  }

  .mark.on {
    border-color: var(--accent-bright);
  }

  .mark.on::after {
    content: '';
    width: 0.8rem;
    height: 0.8rem;
    border-radius: 50%;
    background: var(--accent);
  }

  .body {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .title,
  .note,
  .reason {
    overflow-wrap: anywhere;
  }

  .title {
    font-size: var(--font-sm);
  }

  .note {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .reason {
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .fact {
    flex-shrink: 0;
    font-size: var(--font-xs);
    font-variant-numeric: tabular-nums;
    color: var(--text-3);
  }
</style>
