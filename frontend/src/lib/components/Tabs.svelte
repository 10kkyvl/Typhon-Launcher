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
</script>

<div class="tabs {variant}" role="tablist">
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

  .underline .tab.selected::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
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
    background: var(--surface-3);
    border-color: var(--border);
    color: var(--text);
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
