<script lang="ts">
  import DownloadItem from '../components/DownloadItem.svelte';
  import type { Download } from '../services/downloads';
  import { active } from '../stores/downloads';

  // The active section as the Downloads page mounts it: one panel per download of the store's `active`
  // list, keyed by id, under a heading that takes focus when the last panel goes. For
  // DownloadItem.dom.test.ts.
  let {
    onopen,
    onannounce,
  }: {
    onopen?: (download: Download) => void;
    onannounce?: (text: string) => void;
  } = $props();

  let heading = $state<HTMLElement>();
</script>

<section>
  <h2 id="active-heading" tabindex="-1" bind:this={heading}>Active</h2>
  {#each $active as download (download.id)}
    <DownloadItem {download} {onopen} {onannounce} away={() => heading} />
  {/each}
</section>
