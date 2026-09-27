<script lang="ts">
  let {
    tabs,
    value = $bindable(),
    variant = 'underline',
  }: {
    tabs: { id: string; label: string; count?: number }[];
    value: string;
    variant?: 'underline' | 'pill';
  } = $props();

  let indicator = $state({ left: 0, width: 0, ready: false });

  function trackSelection(node: HTMLDivElement, _value: string) {
    let frame = 0;
    const measure = () => {
      const selected = node.querySelector<HTMLButtonElement>('[aria-selected="true"]');
      indicator = selected
        ? { left: selected.offsetLeft, width: selected.offsetWidth, ready: true }
        : { left: 0, width: 0, ready: false };
    };
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(measure);
    };
    const resize = new ResizeObserver(schedule);
    const observeSizes = () => {
      resize.disconnect();
      resize.observe(node);
      node.querySelectorAll('button').forEach((button) => resize.observe(button));
      schedule();
    };
    const mutation = new MutationObserver(observeSizes);
    mutation.observe(node, { childList: true, subtree: true, characterData: true });
    observeSizes();
    return {
      update: schedule,
      destroy() {
        cancelAnimationFrame(frame);
        resize.disconnect();
        mutation.disconnect();
      },
    };
  }
</script>

<div class="tabs {variant}" role="tablist" use:trackSelection={value}>
  <span
    class="indicator"
    class:ready={indicator.ready}
    aria-hidden="true"
    style:width="{indicator.width}px"
    style:transform="translateX({indicator.left}px)"
  ></span>
  {#each tabs as tab (tab.id)}
    <button
      role="tab"
      aria-selected={value === tab.id}
      class="tab"
      class:selected={value === tab.id}
      onclick={() => (value = tab.id)}
    >
      {tab.label}
      {#if tab.count !== undefined}
        <span class="count">{tab.count}</span>
      {/if}
    </button>
  {/each}
</div>

<style>
  .tabs {
    position: relative;
    isolation: isolate;
    display: flex;
    gap: var(--space-2);
  }

  .tabs.underline {
    border-bottom: 1px solid var(--border);
  }

  .tab {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 0.7rem;
    font-size: var(--font-md);
    font-weight: 500;
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  .underline .tab {
    padding: 0.8rem 0.4rem 1.1rem;
    margin-right: var(--space-3);
  }

  .underline .tab:hover {
    color: var(--text-2);
  }

  .underline .tab.selected {
    color: var(--text);
  }

  .indicator {
    position: absolute;
    left: 0;
    pointer-events: none;
    opacity: 0;
    z-index: -1;
  }

  .indicator.ready {
    opacity: 1;
    transition: transform var(--dur-panel) var(--ease), width var(--dur-panel) var(--ease);
  }

  .underline .indicator {
    bottom: -1px;
    height: 2px;
    border-radius: 2px;
    background: var(--accent);
  }

  .pill .tab {
    height: var(--control-sm);
    padding: 0 var(--space-3) 0 var(--space-4);
    border-radius: var(--radius-xl);
    border: 1px solid transparent;
  }

  .pill .tab:hover {
    color: var(--text-2);
  }

  .pill .tab.selected {
    color: var(--text);
  }

  .pill .indicator {
    top: 0;
    bottom: 0;
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
