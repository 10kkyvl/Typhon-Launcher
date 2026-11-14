<script lang="ts">
  import Card from '../../lib/components/Card.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { AccountError } from '../../lib/services/account';
  import type { Summary } from '../../lib/services/reviews';
  import { reviewScore, reviewScoreBadgeKind } from '../../lib/game/reviewScore';
  import { firstReviewsPage } from '../../lib/game/reviewsFeed';
  import { reviewsChanged } from '../../lib/game/reviewsRefresh';
  import { msg } from '../../lib/i18n';

  let { canonicalGameId, onopen }: { canonicalGameId: string; onopen?: () => void } = $props();

  let summary = $state<Summary | null>(null);
  let loading = $state(false);
  let failed = $state(false);
  let unsupported = $state(false);
  let retrying = $state(false);
  let scoreButton = $state<HTMLButtonElement>();
  let loadToken = 0;
  let shownFor = '';

  // A retry keeps the failure on screen until its answer: the notice and its focused button stay.
  async function load(id: string, retry = false) {
    const token = ++loadToken;
    loading = true;
    if (!retry) failed = false;
    try {
      const page = await firstReviewsPage(id, 'helpful', 'all');
      if (token !== loadToken) return;
      summary = page.summary;
      unsupported = false;
      failed = false;
    } catch (err) {
      if (token !== loadToken) return;
      summary = null;
      if (err instanceof AccountError && err.code === 'unknown_game') {
        unsupported = true;
        failed = false;
      } else failed = true;
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  async function retry(): Promise<boolean> {
    await load(canonicalGameId, true);
    return !failed;
  }

  $effect(() => {
    const id = canonicalGameId;
    void $reviewsChanged;
    if (!id) {
      loadToken += 1;
      summary = null;
      failed = false;
      unsupported = false;
      loading = false;
      shownFor = '';
      return;
    }
    // Another game: the previous one's score, or its being unknown to the service, is not this one's.
    if (shownFor !== id) {
      summary = null;
      unsupported = false;
    }
    shownFor = id;
    void load(id);
    return () => {
      loadToken += 1;
    };
  });

  const score = $derived(summary ? reviewScore(summary.total, summary.positive) : null);
  const line = $derived(
    score && summary
      ? msg('reviews.summaryLine', { percent: score.percent, reviews: msg('reviews.reviewsCount', { count: summary.total }) })
      : '',
  );
</script>

{#if canonicalGameId && !unsupported}
  <Card title={msg('reviews.summaryTitle')}>
    {#if failed || retrying}
      <Notice
        tone="danger"
        compact
        announce={false}
        bind:retrying
        retry={{ label: msg('common.retry'), run: retry, focus: () => (onopen ? scoreButton : undefined) }}
      >
        {msg('reviews.summaryError')}
      </Notice>
    {:else if loading && !summary}
      <Spinner size="sm" label={msg('reviews.summaryLoading')} />
    {:else if score}
      <button class="score" type="button" disabled={!onopen} onclick={() => onopen?.()} bind:this={scoreButton}>
        <StatusBadge kind={reviewScoreBadgeKind(score.tone)} label={score.label} />
        {#if summary && summary.total > 0}
          <span class="line">{line}</span>
        {/if}
      </button>
    {/if}
  </Card>
{/if}

<style>
  .score {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.6rem;
    width: 100%;
    text-align: left;
    border-radius: var(--radius-md);
  }

  .score:hover:not(:disabled) .line {
    color: var(--text);
  }

  .score:disabled {
    cursor: default;
  }

  .line {
    font-size: var(--font-xs);
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }
</style>
