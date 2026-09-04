<script lang="ts">
  import { acceptSourcesNotice } from '../stores/sourcesNotice';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    mode = 'gate',
    onaccepted,
  }: {
    open?: boolean;
    mode?: 'gate' | 'review';
    onaccepted?: () => void;
  } = $props();

  let saving = $state(false);

  async function confirm() {
    saving = true;
    try {
      const accepted = await acceptSourcesNotice();
      if (!accepted) return;
      open = false;
      onaccepted?.();
    } finally {
      saving = false;
    }
  }

  function cancel() {
    open = false;
  }
</script>

<Modal
  bind:open
  title={mode === 'review' ? msg('modals.sourcesNoticeReviewTitle') : msg('modals.sourcesNoticeGateTitle')}
  width="48rem"
>
  <p class="notice-text">
    {msg('modals.sourcesNoticeBody')}
  </p>
  {#snippet footer()}
    {#if mode === 'review'}
      <Button onclick={cancel}>{msg('common.close')}</Button>
    {:else}
      <Button onclick={cancel}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={saving} onclick={confirm}>
        {saving ? msg('modals.sourcesNoticeSaving') : msg('modals.sourcesNoticeContinue')}
      </Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .notice-text {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
    max-width: var(--prose-max);
  }
</style>
