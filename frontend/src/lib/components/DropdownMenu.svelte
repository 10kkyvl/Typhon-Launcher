<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Check } from '@lucide/svelte';
  import { clickOutside } from '../utils/clickOutside';

  export interface MenuItem {
    id: string;
    label: string;
    danger?: boolean;
    separator?: boolean;
    checked?: boolean;
    dot?: 'online' | 'away' | 'busy' | 'offline';
  }

  let {
    items,
    align = 'right',
    placement = 'down',
    onselect,
    trigger,
  }: {
    items: MenuItem[];
    align?: 'left' | 'right';
    placement?: 'down' | 'up';
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
    <div
      class="menu"
      class:up={placement === 'up'}
      style:left={align === 'left' ? '0' : 'auto'}
      style:right={align === 'right' ? '0' : 'auto'}
    >
      {#each items as item (item.id)}
        {#if item.separator}
          <div class="separator"></div>
        {/if}
        <button class="item" class:danger={item.danger} onclick={() => pick(item.id)}>
          {#if item.dot}<span class="dot {item.dot}"></span>{/if}
          <span class="label">{item.label}</span>
          {#if item.checked}<Check size="1.5rem" strokeWidth={2.2} />{/if}
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
    top: calc(100% + 0.4rem);
    min-width: 19rem;
    padding: 0.4rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
  }

  .menu.up {
    top: auto;
    bottom: calc(100% + 0.4rem);
    animation: pop-up var(--dur-fast) var(--ease);
  }

  .item {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    width: 100%;
    padding: 0.7rem 1rem;
    border-radius: var(--radius-sm);
    font-size: var(--font-sm);
    color: var(--text-2);
    text-align: left;
    white-space: nowrap;
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .item:hover {
    background: var(--hover-strong);
    color: var(--text);
  }

  .label {
    flex: 1;
    min-width: 0;
  }

  .item :global(svg) {
    flex-shrink: 0;
    color: var(--text-3);
  }

  .dot {
    width: 0.8rem;
    height: 0.8rem;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .dot.online {
    background: var(--success);
  }

  .dot.away {
    background: var(--warning);
  }

  .dot.busy {
    background: var(--danger);
  }

  .dot.offline {
    background: var(--text-3);
  }

  .item.danger {
    color: var(--danger);
  }

  .item.danger:hover {
    background: var(--danger-subtle);
  }

  .separator {
    height: 1px;
    margin: 0.4rem 0.4rem;
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

  @keyframes pop-up {
    from {
      opacity: 0;
      transform: translateY(0.3rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
