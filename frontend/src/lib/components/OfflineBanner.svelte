<script lang="ts">
  import { Activity, RefreshCw, WifiOff } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
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
      console.error('open status page', err);
      toast(msg('tickets.statusOpenFailed'), 'danger');
    }
  }
</script>

{#if !dismissed}
  <div class="offline-banner">
    <Notice tone="warning" onclose={() => (dismissed = true)}>
      {#snippet icon()}
        <WifiOff size="1.6rem" strokeWidth={1.8} />
      {/snippet}
      {msg('ui.offlineBannerText')}
      {#snippet action()}
        <Button size="sm" variant="ghost" onclick={openStatus}>
          <Activity size="1.4rem" strokeWidth={1.8} />
          {msg('ui.offlineBannerStatus')}
        </Button>
        <Button size="sm" busy={retrying} onclick={retry}>
          {#if !retrying}<RefreshCw size="1.4rem" strokeWidth={1.8} />{/if}
          {retrying ? msg('ui.checkingConnection') : msg('common.retry')}
        </Button>
      {/snippet}
    </Notice>
  </div>
{/if}

<style>
  .offline-banner {
    margin: 0 var(--page-x) var(--space-3);
    flex-shrink: 0;
  }
</style>
