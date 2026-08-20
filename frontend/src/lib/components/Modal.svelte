<script lang="ts">
  import type { Snippet } from 'svelte';
  import { X } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';

  let {
    open = $bindable(false),
    title,
    width = '46rem',
    children,
    footer,
  }: {
    open?: boolean;
    title: string;
    width?: string;
    children?: Snippet;
    footer?: Snippet;
  } = $props();

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') open = false;
  }
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

{#if open}
  <div class="overlay" role="presentation" onpointerdown={(e) => e.target === e.currentTarget && (open = false)}>
    <div class="modal" style:width role="dialog" aria-modal="true" aria-label={title}>
      <div class="head">
        <h3>{title}</h3>
        <IconButton label="Закрыть" size="sm" onclick={() => (open = false)}>
          <X size="1.7rem" strokeWidth={1.8} />
        </IconButton>
      </div>
      <div class="body">
        {@render children?.()}
      </div>
      {#if footer}
        <div class="foot">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(4, 6, 10, 0.6);
    backdrop-filter: blur(0.3rem);
    animation: fade var(--dur) var(--ease);
  }

  .modal {
    max-width: calc(100vw - 4.8rem);
    max-height: calc(100vh - 8rem);
    display: flex;
    flex-direction: column;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-modal);
    animation: rise var(--dur) var(--ease);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1.8rem 2rem 0 2.4rem;
  }

  h3 {
    font-size: 1.8rem;
    font-weight: 600;
  }

  .body {
    padding: 1.6rem 2.4rem 2.4rem;
    overflow-y: auto;
  }

  .foot {
    display: flex;
    justify-content: flex-end;
    gap: 1rem;
    padding: 1.6rem 2.4rem;
    border-top: 1px solid var(--border);
  }

  @keyframes fade {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }

  @keyframes rise {
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
