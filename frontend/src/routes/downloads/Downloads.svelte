<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { Download, Plus } from '@lucide/svelte';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DownloadDetailsModal from '../../lib/components/DownloadDetailsModal.svelte';
  import DownloadItem from '../../lib/components/DownloadItem.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import InstallModal from '../../lib/components/InstallModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { active, completed, failed, queue } from '../../lib/stores/downloads';
  import { requestArt } from '../../lib/stores/metadata';
  import { msg } from '../../lib/i18n';
  import CompletedList from './CompletedList.svelte';
  import DownloadsActions from './DownloadsActions.svelte';
  import DownloadsAnnouncer from './DownloadsAnnouncer.svelte';
  import DownloadsSection from './DownloadsSection.svelte';
  import DownloadsSummary from './DownloadsSummary.svelte';
  import FailedList from './FailedList.svelte';
  import PlayThrottleBanner from './PlayThrottleBanner.svelte';
  import QueueList from './QueueList.svelte';
  import { downloadsSectionIds } from './sections';

  let addOpen = $state(false);
  let detailsOpen = $state(false);
  let detailsId = $state<string | null>(null);
  let installOpen = $state(false);
  let installDownloadId = $state<string | null>(null);
  let announcer = $state<DownloadsAnnouncer>();

  const nothing = $derived(
    $active.length === 0 && $queue.length === 0 && $completed.length === 0 && $failed.length === 0,
  );

  /** What the lists say in the screen's live region (`onannounce`): what the player did, never progress. */
  function announce(text: string) {
    announcer?.announce(text);
  }

  function openDetails(id: string) {
    detailsId = id;
    detailsOpen = true;
  }

  function openInstall(id: string) {
    installDownloadId = id;
    installOpen = true;
  }

  $effect(() => {
    const ids = [...$queue, ...$failed, ...$completed]
      .map((d) => d.origin.gameId)
      .filter((id): id is string => Boolean(id));
    if (ids.length > 0) requestArt(ids);
  });
</script>

<Page>
  <PageHeader title={msg('transfers.downloadsTitle')}>
  {#snippet meta()}
    <DownloadsSummary />
  {/snippet}
  {#snippet actions()}
    <DownloadsActions onadd={() => (addOpen = true)} />
  {/snippet}
  </PageHeader>

  <PlayThrottleBanner />

  {#if nothing}
  <EmptyState level={2}
    title={msg('transfers.downloadsEmptyActiveTitle')}
    description={msg('transfers.downloadsEmptyActiveDescription')}
  >
    {#snippet icon()}
      <Download size="2rem" strokeWidth={1.8} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={() => (addOpen = true)}>
        <Plus size="1.5rem" strokeWidth={2} />
        {msg('transfers.downloadsAddAction')}
      </Button>
    {/snippet}
  </EmptyState>
  {:else}
  <DownloadsSection id={downloadsSectionIds.active} title={msg('transfers.downloadsActiveHeading')} count={$active.length}>
    {#if $active.length === 0}
      <EmptyState size="inline" title={msg('transfers.downloadsEmptyActiveTitle')} />
    {:else}
      <div class="rows">
        {#each $active as download (download.id)}
          <DownloadItem {download} onopen={(d) => openDetails(d.id)} />
        {/each}
      </div>
    {/if}
  </DownloadsSection>
  {/if}

  <FailedList onopen={openDetails} />

  <QueueList />

  <CompletedList onopen={openDetails} oninstall={openInstall} />

  <DownloadsAnnouncer bind:this={announcer} />

  <AddDownloadModal bind:open={addOpen} />

  <DownloadDetailsModal bind:open={detailsOpen} id={detailsId} />

  <InstallModal bind:open={installOpen} downloadId={installDownloadId} />
</Page>

<style>
  .rows {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }
</style>
