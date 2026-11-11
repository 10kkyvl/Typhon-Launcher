<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Plate } from '../motion/plate';
  import { nextIndex } from '../utils/roving';
  import Tooltip from './Tooltip.svelte';
  import type { SegmentOption } from './segmented';

  /**
   * One choice of a small set (WAI-ARIA radio group): the view of a list, a mode, a filter of a list.
   * The group needs a name: `labelledby` (the id of the visible label next to it) or `label`. The
   * arrow keys move and choose, wrapping at the ends; Tab reaches the chosen option only. A plate
   * slides under the chosen option (motion/plate.ts).
   *
   * `variant="pill"` is the strip of filters over a list, where an option may show a `count`.
   * `item` draws an option itself (an icon); the option's label is then its name and its hint.
   */
  let {
    options,
    value = $bindable(),
    item,
    disabled = false,
    variant = 'box',
    label,
    labelledby,
  }: {
    options: SegmentOption[];
    value: string;
    item?: Snippet<[SegmentOption]>;
    disabled?: boolean;
    variant?: 'box' | 'pill';
    /** The group's name, when no visible label names it. */
    label?: string;
    /** Id of the visible label next to the group. */
    labelledby?: string;
  } = $props();

  let root = $state<HTMLElement>();
  let plateEl = $state<HTMLElement>();
  let plate: Plate | undefined;

  const off = (index: number) => disabled || options[index]?.disabled === true;
  const chosen = $derived(options.findIndex((option) => option.id === value));
  // The one Tab stop: the chosen option, or the first that can be chosen while none is.
  const stop = $derived(chosen >= 0 && !off(chosen) ? chosen : options.findIndex((_, index) => !off(index)));

  const radios = () => [...(root?.querySelectorAll<HTMLButtonElement>('[role="radio"]') ?? [])];
  const chosenRadio = () => root?.querySelector<HTMLElement>('[role="radio"][aria-checked="true"]') ?? null;

  function onkeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const all = radios();
    const next = nextIndex(all.indexOf(document.activeElement as HTMLButtonElement), options.length, event.key, { orientation: 'both', isDisabled: off });
    if (next === null) return;
    event.preventDefault();
    value = options[next].id;
    all[next]?.focus();
  }

  $effect(() => {
    if (!plateEl || !root) return;
    const current = new Plate(plateEl);
    plate = current;
    return () => {
      current.destroy();
      plate = undefined;
    };
  });

  // The plate follows the choice, and sits still on it when an option changes size.
  $effect(() => {
    void value;
    void options;
    if (!root) return;
    plate?.observe([root, ...radios()], chosenRadio);
    plate?.to(chosenRadio());
  });
</script>

<div class="segmented {variant}" role="radiogroup" aria-label={label} aria-labelledby={label ? undefined : labelledby} bind:this={root}>
  <span class="plate" aria-hidden="true" bind:this={plateEl}></span>
  {#each options as option, index (option.id)}
    <!-- A segment drawn by `item` (an icon) shows its label as a tooltip; a text segment shows it already. -->
    <Tooltip text={item ? option.label : ''} describe={false}>
      <button
        type="button"
        class="segment"
        class:selected={value === option.id}
        role="radio"
        aria-checked={value === option.id}
        aria-label={item ? option.label : undefined}
        tabindex={index === stop ? 0 : -1}
        disabled={off(index)}
        onclick={() => (value = option.id)}
        {onkeydown}
      >
        {#if item}
          {@render item(option)}
        {:else}
          {option.label}
        {/if}
        {#if option.count !== undefined}
          <span class="count">{option.count}</span>
        {/if}
      </button>
    </Tooltip>
  {/each}
</div>

<style>
  .segmented {
    position: relative;
    display: inline-flex;
  }

  /* Placed and sized by motion/plate.ts on the chosen segment. */
  .plate {
    position: absolute;
    left: 0;
    top: 0;
    transform-origin: 0 0;
    pointer-events: none;
    transition: opacity var(--motion-fast) var(--ease-out);
  }

  .segment {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  .segment:hover {
    color: var(--text-2);
  }

  .segment.selected {
    color: var(--text);
  }

  .segment:disabled {
    cursor: default;
    opacity: 0.5;
  }

  .segment:disabled:not(.selected):hover {
    color: var(--text-3);
  }

  .box {
    padding: 0.3rem;
    gap: 2px;
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .box .segment {
    height: 3rem;
    min-width: 3.2rem;
    padding: 0 0.8rem;
    border-radius: var(--radius-sm);
    font-size: var(--font-xs);
  }

  .box .plate {
    border-radius: var(--radius-sm);
    background: var(--surface-4);
  }

  .pill {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .pill .segment {
    gap: 0.7rem;
    height: var(--control-sm);
    padding: 0 var(--space-3) 0 var(--space-4);
    border: 1px solid transparent;
    border-radius: var(--radius-xl);
    font-size: var(--font-md);
    font-weight: 500;
  }

  .pill .plate {
    border: 2px solid var(--border);
    border-radius: var(--radius-xl);
    background: var(--surface-3);
  }

  .count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 2.2rem;
    height: 2.2rem;
    padding: 0 0.6rem;
    border-radius: var(--radius-xl);
    background: var(--hover-strong);
    color: var(--text-3);
    font-size: var(--font-xs);
    font-variant-numeric: tabular-nums;
  }

  .segment.selected .count {
    background: var(--surface-4);
    color: var(--text-2);
  }
</style>
