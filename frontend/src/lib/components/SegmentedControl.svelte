<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    options,
    value = $bindable(),
    item,
  }: {
    options: { id: string; label: string }[];
    value: string;
    item?: Snippet<[{ id: string; label: string }]>;
  } = $props();
</script>

<div class="segmented">
  {#each options as option (option.id)}
    <button
      class="segment"
      class:selected={value === option.id}
      aria-label={option.label}
      aria-pressed={value === option.id}
      title={option.label}
      onclick={() => (value = option.id)}
    >
      {#if item}
        {@render item(option)}
      {:else}
        {option.label}
      {/if}
    </button>
  {/each}
</div>

<style>
  .segmented {
    display: inline-flex;
    padding: 0.3rem;
    gap: 2px;
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .segment {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    height: 3rem;
    min-width: 3.2rem;
    padding: 0 0.8rem;
    border-radius: var(--radius-sm);
    font-size: var(--font-xs);
    color: var(--text-3);
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .segment:hover {
    color: var(--text-2);
  }

  .segment.selected {
    background: var(--surface-4);
    color: var(--text);
  }
</style>
