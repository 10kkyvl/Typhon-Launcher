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
    gap: 4px;
    border-bottom: 1px solid var(--border);
  }

  .tab {
    position: relative;
    padding: 10px 14px 12px;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-2);
    transition: color var(--dur) var(--ease);
  }

  .tab:hover {
    color: var(--text);
  }

  .tab.selected {
    color: var(--text);
  }

  .tab.selected::after {
    content: '';
    position: absolute;
    left: 10px;
    right: 10px;
    bottom: -1px;
    height: 2px;
    border-radius: 2px;
    background: var(--accent);
  }
</style>
