<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { downloadBlocked, reasonLabel, reasonTone } from '../../lib/diskusage/usage';
  import { msg } from '../../lib/i18n';
  import type { DownloadUsage } from '../../lib/services/diskusage';
  import { usageSectionIds } from './sections';
  import UsageRow from './UsageRow.svelte';
  import UsageSection from './UsageSection.svelte';
  import { size, total } from './usageSizes';

  // The data of downloads: why a download's data cannot go, its marks, and its Delete only where Go
  // lets it go.
  let {
    downloads,
    totalBytes,
    ondelete,
  }: {
    downloads: DownloadUsage[];
    totalBytes: number;
    /** Asks before deleting the download's data. */
    ondelete: (download: DownloadUsage) => void;
  } = $props();
</script>

<UsageSection
  id={usageSectionIds.downloads}
  title={msg('diskusage.downloadsTitle')}
  count={downloads.length}
  hint={msg('diskusage.downloadsHint')}
  total={total(totalBytes, downloads.some((d) => d.sizeUnknown))}
  empty={downloads.length === 0}
  emptyText={msg('diskusage.downloadsEmpty')}
>
  {#each downloads as download (download.id)}
    {@const blocked = downloadBlocked(download)}
    <UsageRow
      key="downloads:{download.id}"
      title={download.name}
      path={download.root}
      reason={blocked ? reasonLabel(download.reasonCode) : download.seeding ? msg('diskusage.reason.seeding') : null}
      reasonTone={blocked ? reasonTone(download.reasonCode) : 'plain'}
      size={size(download)}
    >
      {#snippet badges()}
        {#if download.seeding}
          <StatusBadge kind="accent" label={msg('diskusage.downloadSeeding')} plain />
        {/if}
        {#if download.installed}
          <StatusBadge kind="success" label={msg('diskusage.downloadInstalled')} plain />
        {/if}
        {#if download.missing}
          <StatusBadge kind="warning" label={msg('diskusage.missing')} plain />
        {/if}
      {/snippet}
      {#snippet actions()}
        {#if !blocked}
          <Button size="sm" variant="danger" data-row-control="delete" onclick={() => ondelete(download)}>
            {msg('diskusage.downloadDelete')}
          </Button>
        {/if}
      {/snippet}
    </UsageRow>
  {/each}
</UsageSection>
