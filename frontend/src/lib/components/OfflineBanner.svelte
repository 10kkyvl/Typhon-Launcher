<script lang="ts">
  import { get } from 'svelte/store';
  import { Activity, WifiOff } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import { isOffline, retryBootstrap } from '../stores/user';
  import { openStatusPage } from '../services/account';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let dismissed = $state(false);

  // The shell takes the banner away once the connection is back.
  async function retry(): Promise<boolean> {
    await retryBootstrap();
    return !get(isOffline);
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
    <Notice
      tone="warning"
      onclose={() => (dismissed = true)}
      retry={{ label: msg('common.retry'), pending: msg('ui.checkingConnection'), run: retry }}
    >
      {#snippet icon()}
        <WifiOff size="1.6rem" strokeWidth={1.8} />
      {/snippet}
      {msg('ui.offlineBannerText')}
      {#snippet action()}
        <Button size="sm" variant="ghost" onclick={openStatus}>
          <Activity size="1.4rem" strokeWidth={1.8} />
          {msg('ui.offlineBannerStatus')}
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
