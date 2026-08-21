<script lang="ts">
  import { Search } from '@lucide/svelte';

  let {
    value = $bindable(''),
    placeholder = 'Поиск',
    shortcut,
    oninput,
  }: {
    value?: string;
    placeholder?: string;
    shortcut?: string;
    oninput?: (value: string) => void;
  } = $props();
</script>

<div class="search">
  <Search size="1.7rem" strokeWidth={1.8} />
  <input type="text" {placeholder} bind:value oninput={() => oninput?.(value)} />
  {#if shortcut}
    <kbd>{shortcut}</kbd>
  {/if}
</div>

<style>
  .search {
    display: flex;
    align-items: center;
    gap: 1rem;
    height: 4.4rem;
    padding: 0 1.2rem 0 1.5rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    color: var(--text-3);
    transition:
      border-color var(--dur) var(--ease),
      background var(--dur) var(--ease);
  }

  .search:hover {
    border-color: var(--border-strong);
  }

  .search:focus-within {
    border-color: rgba(104, 117, 232, 0.55);
    background: var(--surface-3);
  }

  input {
    flex: 1;
    min-width: 0;
    background: none;
    border: none;
    outline: none;
    font-size: var(--font-md);
    color: var(--text);
  }

  input::placeholder {
    color: var(--text-3);
  }

  kbd {
    padding: 2px 0.7rem;
    border-radius: 0.6rem;
    border: 1px solid var(--border);
    background: rgba(255, 255, 255, 0.04);
    font-family: inherit;
    font-size: 1.2rem;
    color: var(--text-3);
  }
</style>
