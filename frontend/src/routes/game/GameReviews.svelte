<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { AccountError } from '../../lib/services/account';
  import { list as listReviews, type Review, type ReviewFilter, type ReviewSort } from '../../lib/services/reviews';
  import { firstReviewsPage } from '../../lib/game/reviewsFeed';
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
  // The list `cursor` continues: a page further is asked only for the game, order and filter it came from.
  let cursorKey = '';

  const listKey = (id: string, currentSort: string, currentFilter: string) => `${id}\n${currentSort}\n${currentFilter}`;

  // A page further may repeat a review the list holds already (one posted meanwhile shifts the pages).
  function appendNew(held: Review[], page: Review[]): Review[] {
    const seen = new Set(held.map((item) => item.id));
    return [...held, ...page.filter((item) => !seen.has(item.id) && seen.add(item.id))];
  }

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

  // A retry keeps the failure on screen until its answer: the notice and its focused button stay.
  async function loadFirst(id: string, currentSort: string, currentFilter: string, retry = false) {
    const token = ++requestToken;
    loading = true;
    loadingMore = false;
    if (!retry) failed = false;
    moreFailed = false;
    try {
      const page = await firstReviewsPage(id, currentSort as ReviewSort, currentFilter as ReviewFilter);
      if (token !== requestToken) return;
      items = appendNew([], page.reviews);
      cursor = page.next;
      cursorKey = listKey(id, currentSort, currentFilter);
      loaded = true;
      failed = false;
      unsupported = false;
    } catch (err) {
      if (token !== requestToken) return;
      items = [];
      cursor = '';
      cursorKey = '';
      loaded = false;
      if (err instanceof AccountError && err.code === 'unknown_game') {
        unsupported = true;
        failed = false;
      } else failed = true;
    } finally {
      if (token === requestToken) loading = false;
    }
  }

  let retrying = $state(false);

  async function retry(): Promise<boolean> {
    await loadFirst(canonicalGameId, sort, filter, true);
    return !failed;
  }

  async function loadMore() {
    if (!cursor || loadingMore || loading) return;
    const id = canonicalGameId;
    if (cursorKey !== listKey(id, sort, filter)) return;
    const token = requestToken;
    loadingMore = true;
    moreFailed = false;
    try {
      const page = await listReviews(id, sort as ReviewSort, filter as ReviewFilter, cursor);
      if (token !== requestToken) return;
      items = appendNew(items, page.reviews);
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
      cursorKey = '';
      loaded = false;
      loading = false;
      failed = false;
      unsupported = false;
      moreFailed = false;
      floor = 0;
      heldFor = '';
      return;
    }
    // Another game: its reviews are not this one's, so the list goes and its load shows instead.
    if (heldFor !== id) {
      items = [];
      cursor = '';
      cursorKey = '';
      loaded = false;
      moreFailed = false;
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
        <SegmentedControl options={sortTabs} label={msg('reviews.sortLabel')} variant="pill" bind:value={sort} />
        <SegmentedControl options={filterTabs} label={msg('reviews.filterLabel')} variant="pill" bind:value={filter} />
      </div>
    </div>

    <div class="composer">
      <ReviewComposer {canonicalGameId} />
    </div>

    <div class="results" bind:this={results} style:min-height={floor > 0 ? `${floor}px` : undefined}>
    {#if (failed || retrying) && items.length === 0}
      <div class="state">
        <Notice tone="danger" bind:retrying retry={{ label: msg('common.retry'), run: retry, within: () => results }}>
          {msg('reviews.loadError')}
        </Notice>
      </div>
    {:else if loading && items.length === 0}
      <div class="state" role="status">
        <Spinner />
        <p class="muted">{msg('reviews.loading')}</p>
      </div>
    {:else if loaded && items.length === 0}
      <EmptyState level={3} title={msg('reviews.emptyTitle')} description={msg('reviews.emptyDescription')} />
    {:else if items.length > 0}
      <div class="list">
        {#each items as review (review.id)}
          <ReviewCard {review} {guest} />
        {/each}
      </div>
      {#if moreFailed}
        <Notice tone="danger" compact>{msg('reviews.loadError')}</Notice>
      {/if}
      {#if cursor}
        <div class="more">
          <Button busy={loadingMore || loading} onclick={loadMore}>
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

  .more {
    display: flex;
    justify-content: center;
    padding-top: var(--space-2);
  }
</style>
