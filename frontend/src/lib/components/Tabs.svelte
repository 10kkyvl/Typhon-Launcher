<script lang="ts">
  let {
    tabs,
    value = $bindable(),
  }: {
    tabs: { id: string; label: string }[];
    value: string;
  } = $props();
</script>

<div class="tabs" role="tablist">
  {#each tabs as tab (tab.id)}
    <button
      role="tab"
      aria-selected={value === tab.id}
      class="tab"
      class:selected={value === tab.id}
      onclick={() => (value = tab.id)}
    >
      {tab.label}
    </button>
  {/each}
</div>

<style>
  .tabs {
    display: flex;
    gap: var(--space-2);
    border-bottom: 1px solid var(--border);
  }

  .tab {
    position: relative;
    padding: 0.8rem 0.4rem 1.1rem;
    margin-right: var(--space-3);
    font-size: var(--font-md);
    font-weight: 500;
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  .tab:hover {
    color: var(--text-2);
  }

  .tab.selected {
    color: var(--text);
  }

  .tab.selected::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    bottom: -1px;
    height: 2px;
    border-radius: var(--cut) 2px 2px var(--cut);
    background: var(--accent);
  }
</style>
