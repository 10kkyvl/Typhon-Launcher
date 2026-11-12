<script lang="ts">
  import { acceptSourcesNotice } from '../stores/sourcesNotice';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { codeText, msg } from '../i18n';

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
  let failure = $state('');

  const saveFailed = (err: unknown) => {
    failure = codeText(err, msg('state.settingsSaveFailed'));
  };

  async function confirm() {
    saving = true;
    failure = '';
    try {
      const accepted = await acceptSourcesNotice(saveFailed);
      if (!accepted) return;
      open = false;
      onaccepted?.();
    } catch (err) {
      console.error('accept sources notice', err);
      saveFailed(err);
    } finally {
      saving = false;
    }
  }

  function cancel() {
    open = false;
  }
</script>

<Modal
  dismissible={!saving}
  bind:open
  title={mode === 'review' ? msg('modals.sourcesNoticeReviewTitle') : msg('modals.sourcesNoticeGateTitle')}
  width="48rem"
>
  <p class="notice-text">
    {msg('modals.sourcesNoticeBody')}
  </p>
  {#if failure}<div class="failure"><Notice tone="danger" compact>{failure}</Notice></div>{/if}
  {#snippet footer()}
    {#if mode === 'review'}
      <Button disabled={saving} onclick={cancel}>{msg('common.close')}</Button>
    {:else}
      <Button disabled={saving} onclick={cancel}>{msg('common.cancel')}</Button>
      <Button variant="primary" busy={saving} onclick={confirm}>
        {saving ? msg('modals.sourcesNoticeSaving') : msg('modals.sourcesNoticeContinue')}
      </Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .failure {
    margin: var(--space-4) 0 0;
  }

  .notice-text {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
    max-width: var(--prose-max);
  }
</style>
