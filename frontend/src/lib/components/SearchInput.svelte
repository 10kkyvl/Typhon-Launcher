<script lang="ts">
  import { Search } from '@lucide/svelte';
  import { msg } from '../i18n';

  let {
    value = $bindable(''),
    placeholder = msg('ui.search'),
    shortcut,
    loading = false,
    oninput,
    onfocus,
    onkeydown,
    input = $bindable<HTMLInputElement | undefined>(undefined),
  }: {
    value?: string;
    placeholder?: string;
    shortcut?: string;
    loading?: boolean;
    oninput?: (value: string) => void;
    onfocus?: () => void;
    onkeydown?: (event: KeyboardEvent) => void;
    input?: HTMLInputElement;
  } = $props();
</script>

<div class="search">
  <Search size="1.6rem" strokeWidth={1.8} />
  <input
    type="text"
    {placeholder}
    bind:value
    bind:this={input}
    oninput={() => oninput?.(value)}
    onfocus={() => onfocus?.()}
    onkeydown={(event) => onkeydown?.(event)}
  />
  {#if loading}
    <span class="spinner" aria-label={msg('ui.search')}></span>
  {:else if shortcut}
    <kbd>{shortcut}</kbd>
  {/if}
</div>

<style>
  .search {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    height: var(--control-md);
    padding: 0 1rem 0 1.2rem;
    background: var(--surface);
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    color: var(--text-3);
    transition:
      border-color var(--dur) var(--ease),
      background var(--dur) var(--ease);
  }

  .search:hover {
    background: var(--surface-2);
  }

  .search:focus-within {
    border-color: var(--accent-ring);
    background: var(--surface-2);
  }

  input {
    flex: 1;
    min-width: 0;
    background: none;
    border: none;
    outline: none;
    font-size: var(--font-sm);
    color: var(--text);
  }

  input::placeholder {
    color: var(--text-3);
  }

  kbd {
    padding: 1px 0.6rem;
    border-radius: var(--radius-xs);
    background: var(--hover-strong);
    font-family: inherit;
    font-size: 1.1rem;
    color: var(--text-3);
  }

  .search:focus-within kbd {
    display: none;
  }

  .spinner {
    width: 1.4rem;
    height: 1.4rem;
    flex-shrink: 0;
    border: 2px solid var(--hover-strong);
    border-top-color: var(--text-3);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
