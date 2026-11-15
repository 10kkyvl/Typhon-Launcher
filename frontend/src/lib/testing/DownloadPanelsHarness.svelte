<script lang="ts">
  import DownloadItem from '../components/DownloadItem.svelte';
  import type { Download } from '../services/downloads';
  import { active } from '../stores/downloads';
  import { pressed } from '../../routes/downloads/rowFocus';
  import { downloadsSectionIds } from '../../routes/downloads/sections';

  // The active section as the Downloads page mounts it: one panel per download of the store's `active`
  // list, keyed by id, under a heading that takes focus when the last panel goes, with the screen's
  // rule for focus as `leave`. For DownloadItem.dom.test.ts.
  let {
    onopen,
    onannounce,
  }: {
    onopen?: (download: Download) => void;
    onannounce?: (text: string) => void;
  } = $props();
</script>

<section aria-labelledby={downloadsSectionIds.active}>
  <h2 id={downloadsSectionIds.active} tabindex="-1">Active</h2>
  {#each $active as download (download.id)}
    <DownloadItem {download} {onopen} {onannounce} leave={pressed} />
  {/each}
</section>
