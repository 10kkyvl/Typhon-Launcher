<script lang="ts">
  import { clickOutside } from '../utils/clickOutside';

  interface Item {
    id: string;
    label: string;
    danger?: boolean;
    separator?: boolean;
  }

  let {
    items,
    x,
    y,
    onselect,
    onclose,
  }: {
    items: Item[];
    x: number;
    y: number;
    onselect: (id: string) => void;
    onclose: () => void;
  } = $props();

  const edge = 8;

  let menu = $state<HTMLDivElement | null>(null);
  let left = $state(0);
  let top = $state(0);
  let placed = $state(false);

  $effect(() => {
    if (!menu) return;
    const width = menu.offsetWidth;
    const height = menu.offsetHeight;
    left = Math.max(edge, Math.min(x, window.innerWidth - width - edge));
    top = Math.max(edge, Math.min(y, window.innerHeight - height - edge));
    placed = true;
  });

  function onKey(event: KeyboardEvent) {
    if (event.key === 'Escape') onclose();
  }

  function pick(id: string) {
    onselect(id);
    onclose();
  }
</script>

<svelte:window onkeydown={onKey} onresize={onclose} onblur={onclose} />

<div
  class="menu"
  role="menu"
  tabindex="-1"
  bind:this={menu}
  style:left="{left}px"
  style:top="{top}px"
  style:visibility={placed ? 'visible' : 'hidden'}
  use:clickOutside={onclose}
  oncontextmenu={(event) => event.preventDefault()}
>
  {#each items as item (item.id)}
    {#if item.separator}
      <div class="separator"></div>
    {/if}
    <button class="item" class:danger={item.danger} role="menuitem" onclick={() => pick(item.id)}>
      {item.label}
    </button>
  {/each}
</div>

<style>
  .menu {
    position: fixed;
    z-index: 90;
    min-width: 20rem;
    padding: 0.4rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
  }

  .item {
    display: block;
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

  .item.danger {
    color: var(--danger);
  }

  .item.danger:hover {
    background: var(--danger-subtle);
  }

  .separator {
    height: 1px;
    margin: 0.4rem;
    background: var(--border);
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: scale(0.98);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }
</style>
