<script lang="ts">
  import Modal from '../../lib/components/Modal.svelte';
  import { AccountError } from '../../lib/services/account';
  import { report as reportReview, type ReportReason } from '../../lib/services/reviews';
  import { reviewToastMessage } from '../../lib/game/reviewMessages';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';

  let { reviewId, onclose }: { reviewId: number; onclose: () => void } = $props();

  let sending = $state(false);

  const REASONS: { value: ReportReason; label: () => string }[] = [
    { value: 'spam', label: () => msg('reviews.reportReasonSpam') },
    { value: 'offensive', label: () => msg('reviews.reportReasonOffensive') },
    { value: 'spoilers', label: () => msg('reviews.reportReasonSpoilers') },
    { value: 'off_topic', label: () => msg('reviews.reportReasonOffTopic') },
    { value: 'other', label: () => msg('reviews.reportReasonOther') },
  ];

  async function submit(reason: ReportReason) {
    if (sending) return;
    sending = true;
    try {
      await reportReview(reviewId, reason);
      toast(msg('reviews.reportSent'), 'success');
      onclose();
    } catch (err) {
      const code = err instanceof AccountError ? err.code : '';
      toast(reviewToastMessage(code, msg('reviews.reportFailed')), 'danger');
    } finally {
      sending = false;
    }
  }
</script>

<Modal open title={msg('reviews.reportTitle')} width="38rem" {onclose}>
  <div class="reasons">
    {#each REASONS as reason (reason.value)}
      <button class="reason" type="button" disabled={sending} onclick={() => submit(reason.value)}>
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

  .reason:hover:not(:disabled) {
    background: var(--hover);
    border-color: var(--border-strong);
  }

  .reason:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
