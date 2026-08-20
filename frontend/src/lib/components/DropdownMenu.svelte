<script lang="ts">
  import type { Snippet } from 'svelte';
  import { clickOutside } from '../utils/clickOutside';

  export interface MenuItem {
    id: string;
    label: string;
    danger?: boolean;
    separator?: boolean;
  }

  let {
    items,
    align = 'right',
    onselect,
    trigger,
  }: {
    items: MenuItem[];
    align?: 'left' | 'right';
    onselect?: (id: string) => void;
    trigger: Snippet<[{ open: boolean; toggle: () => void }]>;
  } = $props();

  let open = $state(false);

  function pick(id: string) {
    open = false;
    onselect?.(id);
  }
</script>

<div class="dropdown" use:clickOutside={() => (open = false)}>
  {@render trigger({ open, toggle: () => (open = !open) })}
  {#if open}
    <div class="menu" style:left={align === 'left' ? '0' : 'auto'} style:right={align === 'right' ? '0' : 'auto'}>
      {#each items as item (item.id)}
        {#if item.separator}
          <div class="separator"></div>
        {/if}
        <button class="item" class:danger={item.danger} onclick={() => pick(item.id)}>
          {item.label}
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .dropdown {
    position: relative;
    display: inline-flex;
  }

  .menu {
    position: absolute;
    z-index: 50;
    top: calc(100% + 0.6rem);
    min-width: 19rem;
    padding: 0.5rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
  }

  .item {
    display: block;
    width: 100%;
    padding: 0.8rem 1.1rem;
    border-radius: 0.7rem;
    font-size: 1.4rem;
    color: var(--text-2);
    text-align: left;
    white-space: nowrap;
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .item:hover {
    background: rgba(255, 255, 255, 0.06);
    color: var(--text);
  }

  .item.danger {
    color: var(--danger);
  }

  .item.danger:hover {
    background: var(--danger-subtle);
  }

  .separator {
    height: 1px;
    margin: 0.5rem 0.4rem;
    background: var(--border);
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(-0.3rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
