<script lang="ts">
  import { CircleCheck, CircleAlert, Info } from '@lucide/svelte';
  import { dismissToast, toasts, type Toast } from '../stores/toasts';

  function act(t: Toast) {
    dismissToast(t.id);
    t.action?.run();
  }
</script>

{#if $toasts.length > 0}
  <div class="toasts">
    {#each $toasts as t (t.id)}
      <div class="toast {t.kind}">
        {#if t.kind === 'success'}
          <CircleCheck size="1.76rem" strokeWidth={1.8} />
        {:else if t.kind === 'danger'}
          <CircleAlert size="1.76rem" strokeWidth={1.8} />
        {:else}
          <Info size="1.76rem" strokeWidth={1.8} />
        {/if}
        <span>{t.message}</span>
        {#if t.action}
          <button class="action" type="button" onclick={() => act(t)}>{t.action.label}</button>
        {/if}
      </div>
    {/each}
  </div>
{/if}

<style>
  .toasts {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.88rem;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 1.1rem;
    max-width: 41.8rem;
    padding: 1.21rem 1.54rem;
    background: var(--surface-4);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    font-size: var(--font-sm);
    color: var(--text);
    animation: slide var(--dur-panel) var(--ease);
  }

  .action {
    flex-shrink: 0;
    padding: 0.44rem 0.99rem;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--hover);
    color: var(--accent-text);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }

  .action:hover,
  .action:focus-visible {
    background: var(--hover-strong);
  }

  .toast :global(svg) {
    flex-shrink: 0;
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
      transform: translateY(0.66rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
