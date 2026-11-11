<script lang="ts">
  import { Settings, WifiOff } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import { msg } from '../i18n';
  import { networkDownTitle, networkReasonText } from '../network/networkText';
  import { networkDown, networkState } from '../stores/network';
  import { navigate } from '../stores/router';

  const state = $derived($networkState);
  const reason = $derived(state ? networkReasonText(state) : '');
</script>

{#if $networkDown && state}
  <div class="network-banner">
    <Notice tone="danger" title={reason ? networkDownTitle(state) : undefined}>
      {#snippet icon()}
        <WifiOff size="1.6rem" strokeWidth={1.8} />
      {/snippet}
      {reason || networkDownTitle(state)}
      {#snippet action()}
        <Button size="sm" onclick={() => navigate('settings', { tab: 'downloads' })}>
          <Settings size="1.4rem" strokeWidth={1.8} />
          {msg('ui.networkBannerSettings')}
        </Button>
      {/snippet}
    </Notice>
  </div>
{/if}

<style>
  .network-banner {
    margin: 0 var(--page-x) var(--space-3);
    flex-shrink: 0;
  }
</style>
