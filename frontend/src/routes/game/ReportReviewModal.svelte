<script lang="ts">
  import Modal from '../../lib/components/Modal.svelte';
  import { AccountError } from '../../lib/services/account';
  import { report as reportReview, type ReportReason } from '../../lib/services/reviews';
  import { reviewToastMessage } from '../../lib/game/reviewMessages';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';

  let { reviewId, onclose }: { reviewId: number; onclose: () => void } = $props();

  // The reason being sent: its button is busy and keeps focus, the others refuse presses meanwhile.
  let sending = $state<ReportReason | ''>('');

  const REASONS: { value: ReportReason; label: () => string }[] = [
    { value: 'spam', label: () => msg('reviews.reportReasonSpam') },
    { value: 'offensive', label: () => msg('reviews.reportReasonOffensive') },
    { value: 'spoilers', label: () => msg('reviews.reportReasonSpoilers') },
    { value: 'off_topic', label: () => msg('reviews.reportReasonOffTopic') },
    { value: 'other', label: () => msg('reviews.reportReasonOther') },
  ];

  async function submit(reason: ReportReason) {
    if (sending) return;
    sending = reason;
    try {
      await reportReview(reviewId, reason);
      toast(msg('reviews.reportSent'), 'success');
      onclose();
    } catch (err) {
      const code = err instanceof AccountError ? err.code : '';
      toast(reviewToastMessage(code, msg('reviews.reportFailed')), 'danger');
    } finally {
      sending = '';
    }
  }
</script>

<Modal dismissible={!sending} open title={msg('reviews.reportTitle')} width="38rem" {onclose}>
  <div class="reasons">
    {#each REASONS as reason (reason.value)}
      <button
        class="reason"
        type="button"
        aria-busy={sending === reason.value || undefined}
        aria-disabled={sending && sending !== reason.value ? 'true' : undefined}
        onclick={() => submit(reason.value)}
      >
        {reason.label()}
      </button>
    {/each}
  </div>
</Modal>

<style>
  .reasons {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .reason {
    padding: 1rem var(--space-4);
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    text-align: left;
    font-size: var(--font-sm);
    transition: background var(--dur) var(--ease), border-color var(--dur) var(--ease);
  }

  .reason:hover:not([aria-disabled='true'], [aria-busy='true']) {
    background: var(--hover);
    border-color: var(--border-strong);
  }

  .reason[aria-disabled='true'] {
    opacity: 0.5;
    cursor: default;
  }

  .reason[aria-busy='true'] {
    cursor: progress;
  }
</style>
