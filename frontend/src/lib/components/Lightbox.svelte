<script lang="ts">
  import { ChevronLeft, ChevronRight, X } from '@lucide/svelte';
  import { stepIndex } from '../game/view';
  import IconButton from './IconButton.svelte';

  export interface LightboxItem {
    id: string;
    url: string;
  }

  let {
    open = $bindable(false),
    index = $bindable(0),
    items,
    label = 'Просмотр изображения',
  }: {
    open?: boolean;
    index?: number;
    items: LightboxItem[];
    label?: string;
  } = $props();

  let failed = $state<Record<string, boolean>>({});

  const current = $derived(items[index]);
  const broken = $derived(current ? Boolean(failed[current.id]) : false);

  function step(delta: number) {
    index = stepIndex(index, items.length, delta);
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      open = false;
      return;
    }
    if (items.length < 2) return;
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      step(1);
    } else if (event.key === 'ArrowLeft') {
      event.preventDefault();
      step(-1);
    }
  }
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

{#if open && current}
  <div
    class="backdrop"
    role="presentation"
    onpointerdown={(e) => e.target === e.currentTarget && (open = false)}
  >
    <div class="frame" role="dialog" aria-modal="true" aria-label={label}>
      {#if broken}
        <p class="broken">Изображение недоступно</p>
      {:else}
        <img src={current.url} alt="" onerror={() => (failed = { ...failed, [current.id]: true })} />
      {/if}
    </div>

    {#if items.length > 1}
      <div class="nav prev">
        <IconButton label="Предыдущее" onclick={() => step(-1)}>
          <ChevronLeft size="2.4rem" strokeWidth={1.6} />
        </IconButton>
      </div>
      <div class="nav next">
        <IconButton label="Следующее" onclick={() => step(1)}>
          <ChevronRight size="2.4rem" strokeWidth={1.6} />
        </IconButton>
      </div>
      <span class="counter">{index + 1} / {items.length}</span>
    {/if}

    <div class="close">
      <IconButton label="Закрыть" onclick={() => (open = false)}>
        <X size="2rem" strokeWidth={1.8} />
      </IconButton>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 110;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 6rem;
    background: rgba(4, 6, 10, 0.9);
    animation: fade var(--dur-panel) var(--ease);
  }

  .frame {
    max-width: 100%;
    max-height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: none;
  }

  img {
    max-width: 100%;
    max-height: calc(100vh - 12rem);
    width: auto;
    height: auto;
    object-fit: contain;
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-modal);
    animation: rise var(--dur-panel) var(--ease);
  }

  .broken {
    padding: var(--space-8);
    color: var(--text-3);
    font-size: var(--font-sm);
  }

  .nav {
    position: absolute;
    top: 50%;
    transform: translateY(-50%);
  }

  .prev {
    left: 1.6rem;
  }

  .next {
    right: 1.6rem;
  }

  .close {
    position: absolute;
    top: 1.6rem;
    right: 1.6rem;
  }

  .counter {
    position: absolute;
    bottom: 2rem;
    left: 50%;
    transform: translateX(-50%);
    font-size: var(--font-xs);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  @keyframes fade {
    from {
      opacity: 0;
    }
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: scale(0.98);
    }
  }
</style>
