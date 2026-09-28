<script lang="ts">
  import Card from '../../lib/components/Card.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { AccountError } from '../../lib/services/account';
  import { list as listReviews, type Summary } from '../../lib/services/reviews';
  import { reviewScore, reviewScoreBadgeKind } from '../../lib/game/reviewScore';
  import { reviewsChanged } from '../../lib/game/reviewsRefresh';
  import { msg } from '../../lib/i18n';

  let { canonicalGameId, onopen }: { canonicalGameId: string; onopen?: () => void } = $props();

  let summary = $state<Summary | null>(null);
  let loading = $state(false);
  let failed = $state(false);
  let unsupported = $state(false);

  $effect(() => {
    const id = canonicalGameId;
    void $reviewsChanged;
    if (!id) {
      summary = null;
      failed = false;
      unsupported = false;
      loading = false;
      return;
    }
    let cancelled = false;
    loading = true;
    failed = false;
    listReviews(id, 'helpful', 'all', '')
      .then((page) => {
        if (cancelled) return;
        summary = page.summary;
        unsupported = false;
      })
      .catch((err) => {
        if (cancelled) return;
        summary = null;
        if (err instanceof AccountError && err.code === 'unknown_game') unsupported = true;
        else failed = true;
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
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
    {#if failed}
      <p class="muted error">{msg('reviews.summaryError')}</p>
    {:else if loading && !summary}
      <p class="muted">{msg('reviews.summaryLoading')}</p>
    {:else if score}
      <button class="score" type="button" onclick={() => onopen?.()}>
        <StatusBadge kind={reviewScoreBadgeKind(score.tone)} label={score.label} />
        {#if summary && summary.total > 0}
          <span class="line">{line}</span>
        {/if}
      </button>
    {/if}
  </Card>
{/if}

<style>
  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .error {
    color: var(--danger);
  }

  .score {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.6rem;
    width: 100%;
    text-align: left;
    border-radius: var(--radius-md);
  }

  .score:hover .line {
    color: var(--text);
  }

  .line {
    font-size: var(--font-xs);
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }
</style>
