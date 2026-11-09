<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    options,
    value = $bindable(),
    item,
    disabled = false,
  }: {
    options: { id: string; label: string }[];
    value: string;
    item?: Snippet<[{ id: string; label: string }]>;
    disabled?: boolean;
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
      {disabled}
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
    padding: 0.33rem;
    gap: 2px;
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .segment {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    height: 3.3rem;
    min-width: 3.52rem;
    padding: 0 0.88rem;
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

  .segment:disabled {
    cursor: default;
    opacity: 0.5;
  }

  .segment:disabled:not(.selected):hover {
    color: var(--text-3);
  }
</style>
