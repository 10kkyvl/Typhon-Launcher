<script lang="ts">
  import { Search } from '@lucide/svelte';

  let {
    value = $bindable(''),
    placeholder = 'Поиск',
    shortcut,
  }: {
    value?: string;
    placeholder?: string;
    shortcut?: string;
  } = $props();
</script>

<div class="search">
  <Search size={17} strokeWidth={1.8} />
  <input type="text" {placeholder} bind:value />
  {#if shortcut}
    <kbd>{shortcut}</kbd>
  {/if}
</div>

<style>
  .search {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 40px;
    padding: 0 12px 0 14px;
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
    font-size: 14px;
    color: var(--text);
  }

  input::placeholder {
    color: var(--text-3);
  }

  kbd {
    padding: 2px 7px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: rgba(255, 255, 255, 0.04);
    font-family: inherit;
    font-size: 11.5px;
    color: var(--text-3);
  }
</style>
