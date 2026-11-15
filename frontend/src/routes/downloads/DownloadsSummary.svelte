<script lang="ts">
  import { History } from '@lucide/svelte';
  import Reading from '../../lib/components/Reading.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { active, queue, stats } from '../../lib/stores/downloads';
  import { navigate } from '../../lib/stores/router';
  import { locale, t } from '../../lib/i18n';
  import { makeFormat } from '../../lib/utils/format';

  /**
   * The line under the page's title: how many downloads run, are paused and wait, the total speeds
   * while data moves (the store counts only moving downloads), and the way to the history. Not a live
   * region: the speeds change four times a second.
   */
  const fmt = $derived(makeFormat($locale, $t));

  const running = $derived($active.filter((d) => d.status !== 'paused').length);
  const paused = $derived($active.length - running);

  const words = $derived.by(() => {
    const parts: string[] = [];
    if (running) parts.push($t('downloads.page.running', { count: running }));
    if (paused) parts.push($t('downloads.page.paused', { count: paused }));
    if ($queue.length) parts.push($t('downloads.page.queued', { count: $queue.length }));
    if ($stats.downSpeed > 0 || $stats.upSpeed > 0) {
      parts.push($t('downloads.page.speeds', { down: fmt.speedBytes($stats.downSpeed), up: fmt.speedBytes($stats.upSpeed) }));
    }
    return parts.join(' · ');
  });
</script>

{#if words}
  <span class="summary"><Reading text={words} /></span>
{/if}
<TextLink tone="muted" onclick={() => navigate('history')}>
  {#snippet start()}<History />{/snippet}
  {$t('downloads.page.history')}
</TextLink>

<style>
  .summary {
    color: var(--text-2);
  }
</style>
