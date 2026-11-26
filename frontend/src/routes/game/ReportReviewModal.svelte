<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import OptionList from '../../lib/components/OptionList.svelte';
  import { AccountError } from '../../lib/services/account';
  import { report as reportReview, type ReportReason } from '../../lib/services/reviews';
  import { reviewToastMessage } from '../../lib/game/reviewMessages';
  import { toast } from '../../lib/stores/toasts';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { msg } from '../../lib/i18n';

  /**
   * A report of a review, sent with one reason. The reasons are a radio group: choosing one sends
   * nothing (the arrows choose as they move), Send does.
   */
  let { reviewId, onclose }: { reviewId: number; onclose: () => void } = $props();
  const uid = $props.id();
  const failureId = `${uid}-failure`;

  let chosen = $state<ReportReason | ''>('');
  let sending = $state(false);
  // Why the report was refused, in the launcher's words for the code.
  let failure = $state('');

  const REASONS: { id: ReportReason; label: () => string }[] = [
    { id: 'spam', label: () => msg('reviews.reportReasonSpam') },
    { id: 'offensive', label: () => msg('reviews.reportReasonOffensive') },
    { id: 'spoilers', label: () => msg('reviews.reportReasonSpoilers') },
    { id: 'off_topic', label: () => msg('reviews.reportReasonOffTopic') },
    { id: 'other', label: () => msg('reviews.reportReasonOther') },
  ];
  const options = $derived(REASONS.map((reason) => ({ id: reason.id, title: reason.label() })));

  async function submit() {
    if (sending || !chosen) return;
    // The focus Send holds at the press goes to the words of a refusal.
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    sending = true;
    failure = '';
    try {
      await reportReview(reviewId, chosen);
      toast(msg('reviews.reportSent'), 'success');
      onclose();
    } catch (err) {
      failure = reviewToastMessage(err instanceof AccountError ? err.code : '', msg('reviews.reportFailed'));
    } finally {
      sending = false;
    }
    if (failure) await hand(() => document.getElementById(failureId));
  }

  const heldReason = $derived(sending ? msg('modals.actionRunning') : '');
</script>

<Modal dismissible={!sending} busy={sending} open title={msg('reviews.reportTitle')} width="38rem" {onclose}>
  <OptionList label={msg('reviews.reportReasonLabel')} {options} bind:value={chosen} disabled={sending} />
  {#if failure}
    <div class="notice">
      <Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice>
    </div>
  {/if}
  {#snippet footer()}
    <Button disabled={sending} reason={heldReason} onclick={onclose}>{msg('common.cancel')}</Button>
    <Button
      variant="primary"
      disabled={!chosen}
      reason={chosen ? '' : msg('reviews.reportChooseReason')}
      busy={sending}
      onclick={submit}>{msg('reviews.reportSubmit')}</Button
    >
  {/snippet}
</Modal>

<style>
  .notice {
    margin-top: var(--space-3);
  }
</style>
