<script lang="ts">
  import { Settings, WifiOff } from '@lucide/svelte';
  import Button from './Button.svelte';
  import { msg } from '../i18n';
  import { networkDownTitle, networkReasonText } from '../network/networkText';
  import { networkDown, networkState } from '../stores/network';
  import { navigate } from '../stores/router';

  const state = $derived($networkState);
  const reason = $derived(state ? networkReasonText(state) : '');
</script>

{#if $networkDown && state}
  <div class="network-banner" role="alert">
    <span class="icon"><WifiOff size="1.6rem" strokeWidth={1.8} /></span>
    <span class="text">
      <span class="title">{networkDownTitle(state)}</span>
      {#if reason}
        <span class="reason">{reason}</span>
      {/if}
    </span>
    <Button size="sm" onclick={() => navigate('settings', { tab: 'downloads' })}>
      <Settings size="1.4rem" strokeWidth={1.8} />
      {msg('ui.networkBannerSettings')}
    </Button>
  </div>
{/if}

<style>
  .network-banner {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--page-x);
    background: var(--danger-subtle);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .icon {
    display: flex;
    flex-shrink: 0;
    color: var(--danger);
  }

  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: var(--font-xs);
  }

  .title {
    color: var(--text);
    font-weight: 500;
  }

  .reason {
    color: var(--text-2);
  }
</style>
