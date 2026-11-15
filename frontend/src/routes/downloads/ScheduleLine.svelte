<script lang="ts">
  import { CalendarClock } from '@lucide/svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { scheduleOpenLabel } from '../../lib/services/playThrottle';
  import { playThrottle } from '../../lib/stores/playThrottle';
  import { navigate } from '../../lib/stores/router';
  import { settings } from '../../lib/stores/settings';
  import { t } from '../../lib/i18n';

  /**
   * The schedule's status in the page's header: whether it holds downloads now and until when, else
   * when it lets them run. It is the page's one place for the schedule (the play-throttle banner says
   * only what a running game does), and it leads to the editor in Settings.
   */
  const windows = $derived($settings?.downloadSchedule ?? []);

  const text = $derived.by(() => {
    const schedule = $playThrottle.schedule;
    if (schedule?.blocked) {
      const when = scheduleOpenLabel(schedule.nextOpenAt);
      return when ? $t('downloads.page.scheduleHeld', { when }) : $t('downloads.page.scheduleHeldNoNext');
    }
    if (windows.length === 1) return $t('downloads.page.scheduleFrom', { from: windows[0].from });
    if (windows.length > 1) return $t('downloads.page.scheduleWindows', { count: windows.length });
    return '';
  });
</script>

{#if text}
  <span class="schedule">
    <Tooltip text={$t('downloads.page.scheduleOpen')} placement="bottom">
      <TextLink tone="muted" onclick={() => navigate('settings', { tab: 'downloads' })}>
        {#snippet start()}<CalendarClock />{/snippet}
        {text}
      </TextLink>
    </Tooltip>
  </span>
{/if}

<style>
  .schedule {
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
</style>
