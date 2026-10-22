<script lang="ts">
  import { Activity, RefreshCw, WifiOff, X } from '@lucide/svelte';
  import Button from './Button.svelte';
  import { retryBootstrap } from '../stores/user';
  import { openStatusPage } from '../services/account';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let retrying = $state(false);
  let dismissed = $state(false);

  async function retry() {
    if (retrying) return;
    retrying = true;
    try {
      await retryBootstrap();
    } finally {
      retrying = false;
    }
  }

  async function openStatus() {
    try {
      await openStatusPage();
    } catch (err) {
      toast(err instanceof Error ? err.message : String(err), 'danger');
    }
  }
</script>

{#if !dismissed}
  <div class="offline-banner" role="status">
    <span class="icon"><WifiOff size="1.6rem" strokeWidth={1.8} /></span>
    <span class="text">
      {msg('ui.offlineBannerText')}
    </span>
    <Button size="sm" variant="ghost" onclick={openStatus}>
      <Activity size="1.4rem" strokeWidth={1.8} />
      {msg('ui.offlineBannerStatus')}
    </Button>
    <Button size="sm" disabled={retrying} onclick={retry}>
      <RefreshCw size="1.4rem" strokeWidth={1.8} class={retrying ? 'spin' : ''} />
      {retrying ? msg('ui.checkingConnection') : msg('common.retry')}
    </Button>
    <button type="button" class="close" aria-label={msg('common.close')} title={msg('common.close')} onclick={() => (dismissed = true)}>
      <X size="1.4rem" strokeWidth={1.8} />
    </button>
  </div>
{/if}

<style>
  .offline-banner {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--page-x);
    background: var(--warning-subtle);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .icon {
    display: flex;
    flex-shrink: 0;
    color: var(--warning);
  }

  .text {
    flex: 1;
    min-width: 0;
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .close {
    display: flex;
    flex-shrink: 0;
    padding: var(--space-1);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-2);
    cursor: pointer;
  }

  .close:hover {
    color: var(--text);
    background: var(--surface);
  }

  .offline-banner :global(.spin) {
    animation: spin 900ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
