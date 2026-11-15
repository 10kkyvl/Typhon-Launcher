<script lang="ts">
  import BarChart from '../components/BarChart.svelte';
  import type { ChartBucket, ChartScale } from '../components/barChart';

  // A chart whose selection the page holds, with the empty state a caller passes. A new `start` sets
  // the selection again, as a page resetting its choice would.
  let {
    buckets,
    selectable = false,
    start,
    scale,
    values,
    tickFormat,
  }: {
    buckets: ChartBucket[];
    selectable?: boolean;
    start?: string | null;
    scale?: ChartScale;
    values?: 'all' | 'selected';
    tickFormat?: (value: number) => string;
  } = $props();

  // svelte-ignore state_referenced_locally
  let selected = $state(start);
  $effect(() => {
    selected = start;
  });
  const format = (value: number) => (value > 0 ? `${value} ч` : 'не играли');
</script>

{#if !selectable}
  <BarChart label="Время по дням" {buckets} {format} {scale} {values} {tickFormat}>
    {#snippet empty()}<p data-testid="empty">За неделю не играли</p>{/snippet}
  </BarChart>
{:else}
  <BarChart label="Недели" {buckets} {format} {scale} {values} {tickFormat} selectable bind:selected>
    {#snippet empty()}<p data-testid="empty">За неделю не играли</p>{/snippet}
  </BarChart>
  <output data-testid="selected">{selected ?? ''}</output>
{/if}
