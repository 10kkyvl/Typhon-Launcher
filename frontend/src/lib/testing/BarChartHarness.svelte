<script lang="ts">
  import BarChart from '../components/BarChart.svelte';
  import type { ChartBucket, ChartScale } from '../components/barChart';

  // A chart whose selection the page holds, with the empty state a caller passes.
  let {
    buckets,
    start,
    scale,
    values,
  }: { buckets: ChartBucket[]; start?: string; scale?: ChartScale; values?: 'all' | 'selected' } = $props();

  // svelte-ignore state_referenced_locally
  let selected = $state(start);
  const format = (value: number) => (value > 0 ? `${value} ч` : 'не играли');
</script>

{#if start === undefined}
  <BarChart label="Время по дням" {buckets} {format} {scale} {values}>
    {#snippet empty()}<p data-testid="empty">За неделю не играли</p>{/snippet}
  </BarChart>
{:else}
  <BarChart label="Недели" {buckets} {format} {scale} {values} bind:selected>
    {#snippet empty()}<p data-testid="empty">За неделю не играли</p>{/snippet}
  </BarChart>
  <output data-testid="selected">{selected}</output>
{/if}
