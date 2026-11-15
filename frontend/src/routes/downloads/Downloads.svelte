<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { Download, Plus, Store } from '@lucide/svelte';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DownloadDetailsModal from '../../lib/components/DownloadDetailsModal.svelte';
  import DownloadItem from '../../lib/components/DownloadItem.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import InstallModal from '../../lib/components/InstallModal.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { gamePageId } from '../../lib/services/downloads';
  import { claimAmbient } from '../../lib/stores/ambient';
  import { active, completed, downloadsLoaded, downloadsLoadError, failed, queue, reloadDownloads } from '../../lib/stores/downloads';
  import { requestArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { installErrorText } from '../../lib/install/installErrors';
  import { msg, t } from '../../lib/i18n';
  import CompletedSection from './CompletedSection.svelte';
  import DownloadsActions from './DownloadsActions.svelte';
  import DownloadsAnnouncer from './DownloadsAnnouncer.svelte';
  import DownloadsSection from './DownloadsSection.svelte';
  import DownloadsSummary from './DownloadsSummary.svelte';
  import FailedList from './FailedList.svelte';
  import PlayThrottleBanner from './PlayThrottleBanner.svelte';
  import QueueList from './QueueList.svelte';
  import { pressed } from './rowFocus';
  import { downloadsSectionIds } from './sections';

  let addOpen = $state(false);
  let detailsOpen = $state(false);
  let detailsId = $state<string | null>(null);
  let installOpen = $state(false);
  let installDownloadId = $state<string | null>(null);
  let retrying = $state(false);
  let announcer = $state<DownloadsAnnouncer>();

  const nothing = $derived(
    $active.length === 0 && $queue.length === 0 && $completed.length === 0 && $failed.length === 0,
  );
  const loadFailed = $derived(!$downloadsLoaded && $downloadsLoadError != null);
  const loading = $derived(!$downloadsLoaded && !loadFailed);

  // The backdrop shows the first download under way, a running one before a paused one. Only the id is
  // derived, so a progress tick does not claim again.
  const ambientGame = $derived(
    $active.filter((d) => d.status !== 'paused').map(gamePageId).find(Boolean) ||
      $active.map(gamePageId).find(Boolean) ||
      null,
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

  $effect(() => claimAmbient(ambientGame));

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

  {#if loadFailed || retrying}
    <div class="load-failed">
      <Notice
        tone="danger"
        title={$t('downloads.page.loadFailed')}
        retry={{ label: $t('common.retry'), run: reloadDownloads }}
        bind:retrying
      >
        {installErrorText($downloadsLoadError, $t('downloads.page.loadFailedText'))}
      </Notice>
    </div>
  {/if}

  {#if $active.length > 0}
    <DownloadsSection id={downloadsSectionIds.active} title={msg('transfers.downloadsActiveHeading')} count={$active.length}>
      <div class="rows">
        {#each $active as download (download.id)}
          <DownloadItem {download} onopen={(d) => openDetails(d.id)} onannounce={announce} leave={pressed} />
        {/each}
      </div>
    </DownloadsSection>
  {:else if !nothing}
    <p class="calm">
      <span>{$t('downloads.page.calm')}</span>
      <Button size="sm" variant="ghost" onclick={() => navigate('catalog')}>
        <Store size="1.4rem" strokeWidth={1.8} />
        {$t('downloads.page.stores')}
      </Button>
    </p>
  {:else if loading}
    <EmptyState level={2} title={$t('downloads.page.loading')} />
  {:else if !loadFailed && !retrying}
    <EmptyState level={2} title={$t('downloads.page.emptyTitle')} description={$t('downloads.page.emptyText')}>
      {#snippet icon()}
        <Download size="2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => navigate('catalog')}>
          <Store size="1.5rem" strokeWidth={1.8} />
          {$t('downloads.page.stores')}
        </Button>
        <Button onclick={() => (addOpen = true)}>
          <Plus size="1.5rem" strokeWidth={2} />
          {msg('transfers.downloadsAddAction')}
        </Button>
      {/snippet}
    </EmptyState>
  {/if}

  <CompletedSection part="installing" onopen={openDetails} oninstall={openInstall} onannounce={announce} />

  <FailedList onopen={openDetails} onannounce={announce} />

  <QueueList onopen={openDetails} onannounce={announce} />

  <CompletedSection part="completed" onopen={openDetails} oninstall={openInstall} onannounce={announce} />

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

  .calm {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    max-width: 140rem;
    margin-bottom: var(--space-8);
    color: var(--text-2);
  }

  .load-failed {
    max-width: 140rem;
    margin-bottom: var(--space-6);
  }
</style>
