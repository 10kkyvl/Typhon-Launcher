<script lang="ts">
  import { RefreshCw, WifiOff } from '@lucide/svelte';
  import Button from './Button.svelte';
  import { retryBootstrap } from '../stores/user';
  import { msg } from '../i18n';

  let retrying = $state(false);

  async function retry() {
    if (retrying) return;
    retrying = true;
    try {
      await retryBootstrap();
    } finally {
      retrying = false;
    }
  }
</script>

<div class="offline-banner">
  <span class="icon"><WifiOff size="1.6rem" strokeWidth={1.8} /></span>
  <span class="text">
    {msg('ui.offlineBannerText')}
  </span>
  <Button size="sm" disabled={retrying} onclick={retry}>
    <RefreshCw size="1.4rem" strokeWidth={1.8} class={retrying ? 'spin' : ''} />
    {retrying ? msg('ui.checkingConnection') : msg('common.retry')}
  </Button>
</div>

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

  .offline-banner :global(.spin) {
    animation: spin 900ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
