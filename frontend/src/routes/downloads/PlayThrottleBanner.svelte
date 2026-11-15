<script lang="ts">
  import { CalendarClock, Gauge, Pause } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import { playThrottleBanner, scheduleOpenLabel } from '../../lib/services/playThrottle';
  import { playThrottle } from '../../lib/stores/playThrottle';
  import { navigate } from '../../lib/stores/router';
  import { rateLimitLabel } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  const playBanner = $derived(playThrottleBanner($playThrottle));
  const playBannerText = $derived.by(() => {
    if (!playBanner) return '';
    if (playBanner.kind === 'schedule') {
      const when = scheduleOpenLabel(playBanner.nextOpenAt);
      if (playBanner.game) {
        return when ? msg('transfers.scheduleAndPlayBanner', { when }) : msg('transfers.scheduleAndPlayBannerNoNext');
      }
      return when ? msg('transfers.scheduleBanner', { when }) : msg('transfers.scheduleBannerNoNext');
    }
    if (playBanner.kind === 'pause') return msg('transfers.playPausedBanner');
    return msg('transfers.playLimitedBanner', {
      down: rateLimitLabel(playBanner.downloadLimit),
      up: rateLimitLabel(playBanner.uploadLimit),
    });
  });
</script>

{#if playBanner}
<div class="play-banner">
  <Notice tone="info">
    {#snippet icon()}
      {#if playBanner.kind === 'schedule'}
        <CalendarClock size="1.6rem" strokeWidth={1.8} />
      {:else if playBanner.kind === 'pause'}
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
