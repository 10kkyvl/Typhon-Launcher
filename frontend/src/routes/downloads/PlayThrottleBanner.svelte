<script lang="ts">
  import { Gauge, Pause } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import { playThrottleBanner } from '../../lib/services/playThrottle';
  import { playThrottle } from '../../lib/stores/playThrottle';
  import { navigate } from '../../lib/stores/router';
  import { rateLimitLabel } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  // What a running game does to downloads. The schedule is said by the header's schedule line, which
  // stands whenever a schedule exists; the banner leaves it out so the page says it once.
  const playBanner = $derived(playThrottleBanner({ ...$playThrottle, schedule: undefined }));
  const playBannerText = $derived.by(() => {
    if (!playBanner) return '';
    if (playBanner.kind === 'pause') return msg('transfers.playPausedBanner');
    if (playBanner.kind === 'limit') {
      return msg('transfers.playLimitedBanner', {
        down: rateLimitLabel(playBanner.downloadLimit),
        up: rateLimitLabel(playBanner.uploadLimit),
      });
    }
    return '';
  });
</script>

{#if playBanner}
<div class="play-banner">
  <Notice tone="info">
    {#snippet icon()}
      {#if playBanner.kind === 'pause'}
        <Pause size="1.6rem" strokeWidth={1.8} />
      {:else}
        <Gauge size="1.6rem" strokeWidth={1.8} />
      {/if}
    {/snippet}
    {playBannerText}
    {#snippet action()}
      <Button size="sm" onclick={() => navigate('settings', { tab: 'downloads' })}>{msg('transfers.playBannerSettings')}</Button>
    {/snippet}
  </Notice>
</div>
{/if}

<style>
  .play-banner {
    max-width: 140rem;
    margin-bottom: var(--space-6);
  }
</style>
