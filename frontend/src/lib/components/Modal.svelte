<script lang="ts">
  import type { Snippet } from 'svelte';
  import { X } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';

  let {
    open = $bindable(false),
    title,
    width = '460px',
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
          <X size={17} strokeWidth={1.8} />
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
    backdrop-filter: blur(3px);
    animation: fade var(--dur) var(--ease);
  }

  .modal {
    max-width: calc(100vw - 48px);
    max-height: calc(100vh - 80px);
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
    padding: 18px 20px 0 24px;
  }

  h3 {
    font-size: 17px;
    font-weight: 600;
  }

  .body {
    padding: 16px 24px 24px;
    overflow-y: auto;
  }

  .foot {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    padding: 16px 24px;
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
