<script lang="ts">
  import { Flag, ThumbsDown, ThumbsUp } from '@lucide/svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import { AccountError } from '../../lib/services/account';
  import { vote as voteReview, type Review, type ReviewVote } from '../../lib/services/reviews';
  import { reviewToastMessage } from '../../lib/game/reviewMessages';
  import { toast } from '../../lib/stores/toasts';
  import { navigate } from '../../lib/stores/router';
  import { playtime, relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import ReportReviewModal from './ReportReviewModal.svelte';

  let { review, guest }: { review: Review; guest: boolean } = $props();

  const BODY_COLLAPSE_LENGTH = 700;

  interface VoteState {
    helpful: number;
    unhelpful: number;
    myVote: ReviewVote;
  }

  let optimistic = $state<VoteState | null>(null);
  let voting = $state(false);
  let expanded = $state(false);
  let reportOpen = $state(false);

  $effect(() => {
    void review;
    optimistic = null;
  });

  const helpful = $derived(optimistic ? optimistic.helpful : review.helpful);
  const unhelpful = $derived(optimistic ? optimistic.unhelpful : review.unhelpful);
  const myVote = $derived(optimistic ? optimistic.myVote : ((review.myVote as ReviewVote) || ''));

  const edited = $derived(review.updatedAt > review.createdAt);
  const long = $derived(review.body.length > BODY_COLLAPSE_LENGTH);
  const bodyText = $derived(expanded || !long ? review.body : `${review.body.slice(0, BODY_COLLAPSE_LENGTH)}…`);
  const showActions = $derived(!review.mine && !guest);

  async function castVote(value: ReviewVote) {
    if (voting) return;
    const previous = optimistic;
    const prevHelpful = helpful;
    const prevUnhelpful = unhelpful;
    const prevVote = myVote;
    const next = prevVote === value ? '' : value;

    let helpfulDelta = 0;
    let unhelpfulDelta = 0;
    if (prevVote === 'helpful') helpfulDelta -= 1;
    if (prevVote === 'unhelpful') unhelpfulDelta -= 1;
    if (next === 'helpful') helpfulDelta += 1;
    if (next === 'unhelpful') unhelpfulDelta += 1;

    optimistic = { helpful: prevHelpful + helpfulDelta, unhelpful: prevUnhelpful + unhelpfulDelta, myVote: next };
    voting = true;
    try {
      const result = await voteReview(review.id, next);
      optimistic = { helpful: result.helpful, unhelpful: result.unhelpful, myVote: (result.myVote as ReviewVote) || '' };
    } catch (err) {
      optimistic = previous;
      const code = err instanceof AccountError ? err.code : '';
      toast(reviewToastMessage(code, msg('reviews.voteFailed')), 'danger');
    } finally {
      voting = false;
    }
  }
</script>

<article class="review">
  <header class="head">
    <button class="who" type="button" onclick={() => navigate('user', { username: review.author.username })}>
      <Avatar size="sm" name={review.author.displayName || review.author.username} src={review.author.avatarUrl} />
      <span class="who-text">
        <span class="name">{review.author.displayName || review.author.username}</span>
        <span class="meta">
          {relativeDate(review.createdAt)}
          {#if edited}
            · {msg('reviews.edited', { date: relativeDate(review.updatedAt) })}
          {/if}
        </span>
      </span>
    </button>
    <span class="recommend" class:positive={review.recommended} class:negative={!review.recommended}>
      {#if review.recommended}
        <ThumbsUp size="1.5rem" strokeWidth={1.8} />
      {:else}
        <ThumbsDown size="1.5rem" strokeWidth={1.8} />
      {/if}
      {review.recommended ? msg('reviews.recommends') : msg('reviews.notRecommends')}
    </span>
  </header>

  <p class="playtime">{msg('reviews.playtimeAtReview', { time: playtime(review.playtimeSeconds) })}</p>

  <p class="body">{bodyText}</p>
  {#if long}
    <button class="more" type="button" onclick={() => (expanded = !expanded)}>
      {expanded ? msg('reviews.readLess') : msg('reviews.readMore')}
    </button>
  {/if}

  <footer class="foot">
    <div class="helpful">
      <span class="helpful-label">{msg('reviews.helpfulQuestion')}</span>
      {#if showActions}
        <button
          class="vote"
          class:active={myVote === 'helpful'}
          type="button"
          disabled={voting}
          onclick={() => castVote('helpful')}
        >
          {msg('reviews.helpfulYes')} ({helpful})
        </button>
        <button
          class="vote"
          class:active={myVote === 'unhelpful'}
          type="button"
          disabled={voting}
          onclick={() => castVote('unhelpful')}
        >
          {msg('reviews.helpfulNo')} ({unhelpful})
        </button>
      {:else}
        <span class="vote-readonly">{msg('reviews.helpfulYes')} ({helpful})</span>
        <span class="vote-readonly">{msg('reviews.helpfulNo')} ({unhelpful})</span>
      {/if}
    </div>
    {#if showActions}
      <button class="report" type="button" onclick={() => (reportOpen = true)}>
        <Flag size="1.4rem" strokeWidth={1.8} />
        {msg('reviews.report')}
      </button>
    {/if}
  </footer>
</article>

{#if reportOpen}
  <ReportReviewModal reviewId={review.id} onclose={() => (reportOpen = false)} />
{/if}

<style>
  .review {
    padding: var(--space-4) 0;
    border-bottom: 1px solid var(--border);
  }

  .review:last-child {
    border-bottom: none;
  }

  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    text-align: left;
    min-width: 0;
  }

  .who-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .name {
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .recommend {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    flex-shrink: 0;
    font-size: var(--font-xs);
    font-weight: 500;
  }

  .recommend.positive {
    color: var(--accent-text);
  }

  .recommend.negative {
    color: var(--danger);
  }

  .playtime {
    margin-top: var(--space-2);
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .body {
    margin-top: var(--space-3);
    font-size: var(--font-sm);
    line-height: 1.6;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .more {
    margin-top: 0.6rem;
    font-size: var(--font-xs);
    font-weight: 500;
    color: var(--accent-text);
  }

  .more:hover {
    text-decoration: underline;
  }

  .foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-top: var(--space-4);
  }

  .helpful {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .helpful-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .vote,
  .vote-readonly {
    display: inline-flex;
    align-items: center;
    height: var(--control-sm);
    padding: 0 1.2rem;
    border-radius: var(--radius-md);
    border: 1px solid var(--border-strong);
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .vote:hover:not(:disabled) {
    background: var(--hover-strong);
  }

  .vote.active {
    background: var(--accent-subtle);
    border-color: var(--accent);
    color: var(--accent-text);
  }

  .vote:disabled {
    opacity: 0.6;
    cursor: default;
  }

  .report {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .report:hover {
    color: var(--text);
  }
</style>
