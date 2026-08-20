<script lang="ts">
  import { CircleCheck, CircleAlert, Info } from '@lucide/svelte';
  import { toasts } from '../stores/toasts';
</script>

<div class="toasts">
  {#each $toasts as t (t.id)}
    <div class="toast {t.kind}">
      {#if t.kind === 'success'}
        <CircleCheck size={17} strokeWidth={1.8} />
      {:else if t.kind === 'danger'}
        <CircleAlert size={17} strokeWidth={1.8} />
      {:else}
        <Info size={17} strokeWidth={1.8} />
      {/if}
      <span>{t.message}</span>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    z-index: 120;
    right: 24px;
    bottom: 24px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 10px;
    max-width: 380px;
    padding: 12px 16px;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    font-size: 13.5px;
    color: var(--text);
    animation: slide var(--dur) var(--ease);
  }

  .toast.success :global(svg) {
    color: var(--success);
  }

  .toast.danger :global(svg) {
    color: var(--danger);
  }

  .toast.info :global(svg) {
    color: var(--accent-text);
  }

  @keyframes slide {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
