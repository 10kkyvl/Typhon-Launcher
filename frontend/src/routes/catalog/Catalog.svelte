<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { get } from 'svelte/store';
  import Notice from '../../lib/components/Notice.svelte';
  import RecommendationShelf from '../../lib/components/RecommendationShelf.svelte';
  import Button from '../../lib/components/Button.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import { catalogView } from '../../lib/stores/ui';
  import { msg } from '../../lib/i18n';
  import { createCatalogPageData } from './catalogPageData.svelte';
  import CatalogToolbar from './CatalogToolbar.svelte';
  import CatalogNotes from './CatalogNotes.svelte';
  import CatalogResults from './CatalogResults.svelte';

  // Created first: its effects are the page's, and run before the view switch's below.
  const data = createCatalogPageData();

  let lastView = get(catalogView);
  let viewSwitched = $state(false);

  $effect(() => {
    if ($catalogView === lastView) return;
    lastView = $catalogView;
    viewSwitched = true;
  });

  function endViewSwitch(event: AnimationEvent) {
    if (event.target === event.currentTarget) viewSwitched = false;
  }
</script>

<Page>
  <PageHeader
    title={msg('games.allGamesTitle')}
    count={data.pages.total > 0 ? msg('games.catalogTotalCount', { count: data.pages.total }) : undefined}
  />

  <CatalogToolbar {data} />

  <CatalogNotes {data} />
  {#if data.discoveryVisible}
    <RecommendationShelf title={msg('games.discoveryTitle')} items={data.discovery} loading={data.discoveryLoading || data.preferenceBusy}
      emptyText={msg('games.recommendationEmpty')} onrefresh={data.refreshPicks} ondismiss={(item) => void data.dismiss(item.game)} />
  {/if}

  <CatalogResults {data} {viewSwitched} {endViewSwitch} />

  {#if data.pages.items.length > 0}
    {#if data.autoLoad}<div class="sentinel" bind:this={data.sentinel} aria-hidden="true"></div>{/if}
    <div class="more" bind:this={data.moreSlot}>
      {#if data.pages.failed || data.moreRetrying}
        <!-- A server too old for this sorting refuses every page the same way: no Retry for it. -->
        <Notice
          tone="danger"
          compact
          tabindex={data.pages.backendOutdated ? -1 : undefined}
          data-more-refused={data.pages.backendOutdated || undefined}
          bind:retrying={data.moreRetrying}
          retry={data.pages.backendOutdated ? undefined : { label: msg('games.catalogRetry'), run: data.retryNextPage, focus: data.firstFresh }}
        >
          {data.pages.backendOutdated ? msg('games.catalogBackendOutdated') : msg('games.catalogMoreFailed')}
        </Notice>
      {:else if data.showMoreButton}
        <Button onclick={data.showMore} busy={data.pages.loading}>{msg('games.catalogShowMore')}</Button>
      {:else if data.hasMore && data.pages.appending}
        <Spinner label={msg('games.catalogLoadingMore')} />
      {/if}
      <span class="muted">{msg('games.catalogShownOf', { shown: data.pages.items.length, total: data.pages.total })}</span>
    </div>
  {/if}
</Page>

<style>
  .sentinel { height: 1px; }

  .more {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    justify-content: center;
    padding: var(--space-8) 0 var(--space-4);
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
