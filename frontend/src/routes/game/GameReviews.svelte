<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { AccountError } from '../../lib/services/account';
  import { list as listReviews, type Review, type ReviewFilter, type ReviewSort } from '../../lib/services/reviews';
  import { reviewsChanged } from '../../lib/game/reviewsRefresh';
  import { msg } from '../../lib/i18n';
  import { measureScrollFloor } from '../../lib/utils/scrollFloor';
  import ReviewCard from './ReviewCard.svelte';
  import ReviewComposer from './ReviewComposer.svelte';
  import { authState } from '../../lib/stores/user';

  let { canonicalGameId }: { canonicalGameId: string } = $props();

  let sort = $state('helpful');
  let filter = $state('all');
  let items = $state<Review[]>([]);
  let cursor = $state('');
  let loading = $state(false);
  let loadingMore = $state(false);
  let failed = $state(false);
  let loaded = $state(false);
  let unsupported = $state(false);
  let moreFailed = $state(false);
  let results = $state<HTMLElement>();
  let floor = $state(0);
  let heldFor = '';

  const guest = $derived($authState !== 'authenticated');

  const sortTabs = $derived([
    { id: 'helpful', label: msg('reviews.sortHelpful') },
    { id: 'recent', label: msg('reviews.sortRecent') },
  ]);
  const filterTabs = $derived([
    { id: 'all', label: msg('reviews.filterAll') },
    { id: 'positive', label: msg('reviews.filterPositive') },
    { id: 'negative', label: msg('reviews.filterNegative') },
  ]);

  let requestToken = 0;

  async function loadFirst(id: string, currentSort: string, currentFilter: string) {
    const token = ++requestToken;
    loading = true;
    failed = false;
    moreFailed = false;
    try {
      const page = await listReviews(id, currentSort as ReviewSort, currentFilter as ReviewFilter, '');
      if (token !== requestToken) return;
      items = page.reviews;
      cursor = page.next;
      loaded = true;
      unsupported = false;
    } catch (err) {
      if (token !== requestToken) return;
      items = [];
      cursor = '';
      loaded = false;
      if (err instanceof AccountError && err.code === 'unknown_game') unsupported = true;
      else failed = true;
    } finally {
      if (token === requestToken) loading = false;
    }
  }

  async function loadMore() {
    if (!cursor || loadingMore) return;
    const id = canonicalGameId;
    const token = requestToken;
    loadingMore = true;
    moreFailed = false;
    try {
      const page = await listReviews(id, sort as ReviewSort, filter as ReviewFilter, cursor);
      if (token !== requestToken) return;
      items = [...items, ...page.reviews];
      cursor = page.next;
    } catch {
      if (token !== requestToken) return;
      moreFailed = true;
    } finally {
      if (token === requestToken) loadingMore = false;
    }
  }

  $effect(() => {
    const id = canonicalGameId;
    const currentSort = sort;
    const currentFilter = filter;
    void $reviewsChanged;
    if (!id) {
      requestToken += 1;
      items = [];
      cursor = '';
      loaded = false;
      loading = false;
      failed = false;
      unsupported = false;
      moreFailed = false;
      floor = 0;
      heldFor = '';
      return;
    }
    const node = untrack(() => results);
    floor = heldFor === id && node ? measureScrollFloor(node) : 0;
    heldFor = id;
    void loadFirst(id, currentSort, currentFilter);
  });
</script>

{#if canonicalGameId && !unsupported}
  <section class="section" id="reviews">
    <div class="header">
      <h2 class="heading">{msg('reviews.sectionTitle')}</h2>
      <div class="controls">
        <Tabs tabs={sortTabs} bind:value={sort} variant="pill" />
        <Tabs tabs={filterTabs} bind:value={filter} variant="pill" />
      </div>
    </div>

    <div class="composer">
      <ReviewComposer {canonicalGameId} />
    </div>

    <div class="results" bind:this={results} style:min-height={floor > 0 ? `${floor}px` : undefined}>
    {#if loading && items.length === 0}
      <div class="state">
        <span class="spinner"></span>
        <p class="muted">{msg('reviews.loading')}</p>
      </div>
    {:else if failed && items.length === 0}
      <div class="state">
        <p class="muted error">{msg('reviews.loadError')}</p>
        <Button size="sm" onclick={() => loadFirst(canonicalGameId, sort, filter)}>{msg('common.retry')}</Button>
      </div>
    {:else if loaded && items.length === 0}
      <EmptyState title={msg('reviews.emptyTitle')} description={msg('reviews.emptyDescription')} />
    {:else if items.length > 0}
      <div class="list">
        {#each items as review (review.id)}
          <ReviewCard {review} {guest} />
        {/each}
      </div>
      {#if moreFailed}
        <p class="muted error">{msg('reviews.loadError')}</p>
      {/if}
      {#if cursor}
        <div class="more">
          <Button disabled={loadingMore} onclick={loadMore}>
            {loadingMore ? msg('reviews.loadingMore') : msg('reviews.loadMore')}
          </Button>
        </div>
      {/if}
    {/if}
    </div>
  </section>
{/if}

<style>
  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    margin-top: var(--space-10);
    scroll-margin-top: var(--space-6);
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .heading {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .composer {
    padding: var(--space-4);
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    background: var(--surface-2);
  }

  .results {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .state {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-6) 0;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .error {
    color: var(--danger);
  }

  .more {
    display: flex;
    justify-content: center;
    padding-top: var(--space-2);
  }

  .spinner {
    width: 2rem;
    height: 2rem;
    flex-shrink: 0;
    border-radius: 50%;
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
