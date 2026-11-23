<script lang="ts">
  import BarChart from '../components/BarChart.svelte';
  import DiskMeter from '../components/DiskMeter.svelte';
  import type { ChartBucket } from '../components/barChart';
  import type { MeterSegment } from '../components/diskMeter';

  // A meter or a chart after another part of the page that moves focus in an effect of its own (a
  // dialog opening) in the same update that changes their data. That part is placed first, so its
  // effect runs before theirs: whatever they hand focus to, they must not take it back.
  let { steal = 0, segments, buckets }: { steal?: number; segments?: MeterSegment[]; buckets?: ChartBucket[] } = $props();
  let dialog = $state<HTMLElement>();
  const GB = 1024 ** 3;
</script>

<span
  {@attach () => {
    if (steal > 0) dialog?.focus();
  }}
></span>
{#if segments}
  <DiskMeter label="Диск" {segments} total={1000 * GB} onpick={() => {}} />
{/if}
{#if buckets}
  <BarChart label="Недели" {buckets} format={(value) => `${value} ч`} selectable />
{/if}
<button type="button" bind:this={dialog}>Диалог</button>
