<script lang="ts">
  import { CircleCheck, CircleAlert, Info } from '@lucide/svelte';
  import { toasts } from '../stores/toasts';
</script>

<div class="toasts">
  {#each $toasts as t (t.id)}
    <div class="toast {t.kind}">
      {#if t.kind === 'success'}
        <CircleCheck size="1.7rem" strokeWidth={1.8} />
      {:else if t.kind === 'danger'}
        <CircleAlert size="1.7rem" strokeWidth={1.8} />
      {:else}
        <Info size="1.7rem" strokeWidth={1.8} />
      {/if}
      <span>{t.message}</span>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    z-index: 120;
    right: 2.4rem;
    bottom: 2.4rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 1rem;
    max-width: 38rem;
    padding: 1.2rem 1.6rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    font-size: 1.4rem;
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
      transform: translateY(0.6rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
