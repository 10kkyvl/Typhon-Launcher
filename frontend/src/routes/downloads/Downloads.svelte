<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { ArrowDown, ArrowUp, Download, Plus } from '@lucide/svelte';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DownloadDetailsModal from '../../lib/components/DownloadDetailsModal.svelte';
  import DownloadItem from '../../lib/components/DownloadItem.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import InstallModal from '../../lib/components/InstallModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { active, completed, failed, queue, stats } from '../../lib/stores/downloads';
  import { requestArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { speedBytes } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import CompletedList from './CompletedList.svelte';
  import DownloadsActions from './DownloadsActions.svelte';
  import DownloadsSection from './DownloadsSection.svelte';
  import FailedList from './FailedList.svelte';
  import PlayThrottleBanner from './PlayThrottleBanner.svelte';
  import QueueList from './QueueList.svelte';

  let addOpen = $state(false);
  let detailsOpen = $state(false);
  let detailsId = $state<string | null>(null);
  let installOpen = $state(false);
  let installDownloadId = $state<string | null>(null);

  const nothing = $derived(
    $active.length === 0 && $queue.length === 0 && $completed.length === 0 && $failed.length === 0,
  );

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
  <div class="summary">
    <span class="sum speed"><ArrowDown size="1.5rem" strokeWidth={2} />{speedBytes($stats.downSpeed)}</span>
    <span class="sum speed dim"><ArrowUp size="1.5rem" strokeWidth={2} />{speedBytes($stats.upSpeed)}</span>
    <span class="sum-sep" aria-hidden="true"></span>
    <span class="sum dim">{msg('downloads.active', { count: $stats.activeCount })}</span>
    <span class="sum dim">{msg('transfers.downloadsQueuedCount', { count: $queue.length })}</span>
  </div>

  <DownloadsSection title={msg('transfers.downloadsActiveHeading')} count={$active.length}>
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

  <p class="footer-hint">
  {msg('transfers.downloadsFooterHintQuestion')}
  <TextLink onclick={() => navigate('history')}>{msg('transfers.downloadsOpenHistoryLog')}</TextLink>
  </p>

  <AddDownloadModal bind:open={addOpen} />

  <DownloadDetailsModal bind:open={detailsOpen} id={detailsId} />

  <InstallModal bind:open={installOpen} downloadId={installDownloadId} />
</Page>

<style>
  .summary {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    max-width: 140rem;
    margin-bottom: var(--space-6);
    padding: var(--space-3) var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .sum {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-weight: 500;
    color: var(--text);
  }

  .sum.speed {
    min-width: 10.5rem;
  }

  .sum.dim {
    font-weight: 400;
    color: var(--text-2);
  }

  .sum :global(svg) {
    color: var(--accent-text);
  }

  .sum.dim :global(svg) {
    color: var(--text-3);
  }

  .sum-sep {
    width: 1px;
    height: 1.8rem;
    background: var(--border-strong);
  }

  .rows {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .footer-hint {
    margin-top: var(--space-6);
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
