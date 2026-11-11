<script lang="ts">
  import { Plate, scrollToShow } from '../motion/plate';
  import { nextIndex } from '../utils/roving';
  import { panelId, tabId, type TabItem } from './tabs';

  /**
   * Tabs that switch what is shown under them (WAI-ARIA tabs). The page gives the strip an `idPrefix`
   * and puts `tabPanel(idPrefix, value)` (tabs.ts) on the element holding the chosen tab's content,
   * so the tab and its panel name each other. Left and right arrows move to the next tab and show it,
   * wrapping at the ends; Tab goes from the chosen tab into its panel. A strip wider than its room
   * scrolls and keeps the chosen tab in view. The marker is the shared sliding plate (motion/plate.ts).
   *
   * A single choice that filters or sorts a list is not tabs: that is SegmentedControl.
   */
  let {
    tabs,
    value = $bindable(),
    variant = 'underline',
    idPrefix,
    label,
    labelledby,
    divider = true,
  }: {
    tabs: TabItem[];
    value: string;
    variant?: 'underline' | 'pill';
    /** The id the tabs and their panels are built from: unique on the page. */
    idPrefix: string;
    /** The strip's name, when no visible heading names it. */
    label?: string;
    /** Id of the visible heading that names the strip. */
    labelledby?: string;
    /** The line under an underline strip; off where the page draws its own edge. */
    divider?: boolean;
  } = $props();

  let strip = $state<HTMLElement>();
  let plateEl = $state<HTMLElement>();
  let plate: Plate | undefined;

  const off = (index: number) => tabs[index]?.disabled === true;
  const chosen = $derived(tabs.findIndex((tab) => tab.id === value));
  const stop = $derived(chosen >= 0 && !off(chosen) ? chosen : tabs.findIndex((_, index) => !off(index)));

  const buttons = () => [...(strip?.querySelectorAll<HTMLButtonElement>('[role="tab"]') ?? [])];
  const chosenTab = () => strip?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]') ?? null;

  function onkeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const all = buttons();
    const next = nextIndex(all.indexOf(document.activeElement as HTMLButtonElement), tabs.length, event.key, { orientation: 'horizontal', isDisabled: off });
    if (next === null) return;
    event.preventDefault();
    value = tabs[next].id;
    all[next]?.focus();
  }

  $effect(() => {
    if (!plateEl || !strip) return;
    const current = new Plate(plateEl);
    plate = current;
    return () => {
      current.destroy();
      plate = undefined;
    };
  });

  // The marker follows the choice and sits still on it when a tab changes size; the strip scrolls
  // just enough to show the chosen tab.
  $effect(() => {
    void value;
    void tabs;
    if (!strip) return;
    const tab = chosenTab();
    if (tab) strip.scrollLeft = scrollToShow(tab.offsetLeft, tab.offsetWidth, strip.scrollLeft, strip.clientWidth);
    plate?.observe([strip, ...buttons()], chosenTab);
    plate?.to(tab);
  });
</script>

<div class="tabs {variant}" class:divider>
  <div class="strip" role="tablist" aria-label={label} aria-labelledby={label ? undefined : labelledby} bind:this={strip}>
    <span class="plate" aria-hidden="true" bind:this={plateEl}></span>
    {#each tabs as tab, index (tab.id)}
      <button
        type="button"
        role="tab"
        id={tabId(idPrefix, tab.id)}
        aria-selected={value === tab.id}
        aria-controls={value === tab.id ? panelId(idPrefix, tab.id) : undefined}
        tabindex={index === stop ? 0 : -1}
        class="tab"
        class:selected={value === tab.id}
        disabled={tab.disabled}
        onclick={() => (value = tab.id)}
        {onkeydown}
      >
        {tab.label}
        {#if tab.count !== undefined}
          <span class="count">{tab.count}</span>
        {/if}
      </button>
    {/each}
  </div>
</div>

<style>
  .tabs.underline.divider {
    border-bottom: 1px solid var(--border);
  }

  /* The strip scrolls when it is wider than its room, with no bar of its own. */
  .strip {
    position: relative;
    display: flex;
    gap: var(--space-2);
    overflow-x: auto;
    scrollbar-width: none;
  }

  .strip::-webkit-scrollbar {
    display: none;
  }

  /* An underline strip reaches one pixel down, over the divider, where the marker sits: a scrolling
     box would otherwise cut the marker there. */
  .underline .strip {
    margin-bottom: -1px;
    padding-bottom: 1px;
  }

  .tab {
    position: relative;
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    gap: 0.7rem;
    font-size: var(--font-md);
    font-weight: 500;
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  /* The ring is drawn inside the tab: outside it the scrolling strip would cut it. */
  .tab:focus-visible {
    outline-offset: -2px;
  }

  .tab:hover {
    color: var(--text-2);
  }

  .tab.selected {
    color: var(--text);
  }

  .tab:disabled {
    cursor: default;
    opacity: 0.5;
  }

  .tab:disabled:hover {
    color: var(--text-3);
  }

  .underline .tab {
    padding: 0.8rem 0.4rem 1.1rem;
    margin-right: var(--space-3);
  }

  /* Placed and sized by motion/plate.ts on the chosen tab. */
  .plate {
    position: absolute;
    left: 0;
    top: 0;
    transform-origin: 0 0;
    pointer-events: none;
    transition: opacity var(--motion-fast) var(--ease-out);
  }

  .underline .plate::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    bottom: -1px;
    height: 2px;
    border-radius: var(--cut);
    background: var(--accent-bright);
  }

  .pill .tab {
    height: var(--control-sm);
    padding: 0 var(--space-3) 0 var(--space-4);
    border-radius: var(--radius-xl);
    border: 1px solid transparent;
  }

  .pill .plate {
    border-radius: var(--radius-xl);
    border: 2px solid var(--border);
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

  .pill .tab.selected .count {
    background: var(--surface-4);
    color: var(--text-2);
  }
</style>
