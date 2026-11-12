<script lang="ts">
  import { Search } from '@lucide/svelte';
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { msg } from '../i18n';
  import Spinner from './Spinner.svelte';

  /**
   * A search field. Attributes it does not know (a role, `aria-*`, an id) go to its `<input>`, so a
   * caller that builds a combobox on it wires the input without reaching into the component.
   */
  let {
    value = $bindable(''),
    placeholder = msg('ui.search'),
    shortcut,
    loading = false,
    variant = 'field',
    oninput,
    onfocus,
    onkeydown,
    input = $bindable<HTMLInputElement | undefined>(undefined),
    ...rest
  }: {
    value?: string;
    placeholder?: string;
    shortcut?: string;
    loading?: boolean;
    /** `island`: the top bar's look, a pill on the window's backdrop. */
    variant?: 'field' | 'island';
    oninput?: (value: string) => void;
    onfocus?: () => void;
    onkeydown?: (event: KeyboardEvent) => void;
    input?: HTMLInputElement;
  } & Omit<HTMLInputAttributes, 'value' | 'type' | 'placeholder' | 'class' | 'oninput' | 'onfocus' | 'onkeydown'> = $props();
</script>

<div class="search" class:island={variant === 'island'}>
  <Search size="1.6rem" strokeWidth={1.8} />
  <input
    {...rest}
    type="text"
    {placeholder}
    bind:value
    bind:this={input}
    oninput={() => oninput?.(value)}
    onfocus={() => onfocus?.()}
    onkeydown={(event) => onkeydown?.(event)}
  />
  {#if loading}
    <span class="wait"><Spinner size="sm" tone="current" /></span>
  {:else if shortcut}
    <kbd>{shortcut}</kbd>
  {/if}
  <!-- The spinner is a picture: that a search is running is said here. -->
  <span class="sr-only" role="status">{loading ? msg('search.searching') : ''}</span>
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

  .search.island {
    background: var(--island-fill);
    border-color: var(--border-strong);
    border-radius: var(--radius-control);
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

  .wait {
    display: inline-flex;
    flex: none;
    color: var(--text-3);
  }
</style>
