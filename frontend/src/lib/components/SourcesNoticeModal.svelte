<script lang="ts">
  import { acceptSourcesNotice } from '../stores/sourcesNotice';
  import { focusHandOff } from '../utils/focusHandOff';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { codeText, msg } from '../i18n';

  /**
   * The notice about sources, before the first one is added (`gate`) or read again from the settings
   * (`review`). The safe button (Cancel, Close) takes the focus on opening, and Escape does what it
   * does; while Continue saves, both wait and say why. A failed save is said in the dialog by code and
   * takes the focus.
   */
  let {
    open = $bindable(false),
    mode = 'gate',
    onaccepted,
  }: {
    open?: boolean;
    mode?: 'gate' | 'review';
    onaccepted?: () => void;
  } = $props();

  const uid = $props.id();
  const failureId = `${uid}-failure`;

  let saving = $state(false);
  let failure = $state('');

  const saveFailed = (err: unknown) => {
    failure = codeText(err, msg('state.settingsSaveFailed'));
  };

  async function confirm() {
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    saving = true;
    failure = '';
    try {
      const accepted = await acceptSourcesNotice(saveFailed);
      if (accepted) {
        open = false;
        onaccepted?.();
      }
    } catch (err) {
      console.error('accept sources notice', err);
      saveFailed(err);
    } finally {
      saving = false;
    }
    if (failure) await hand(() => document.getElementById(failureId));
  }

  function cancel() {
    open = false;
  }
</script>

<Modal
  dismissible={!saving}
  busy={saving}
  bind:open
  title={mode === 'review' ? msg('modals.sourcesNoticeReviewTitle') : msg('modals.sourcesNoticeGateTitle')}
  width="48rem"
>
  <p class="notice-text">
    {msg('modals.sourcesNoticeBody')}
  </p>
  {#if failure}<div class="failure"><Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice></div>{/if}
  {#snippet footer()}
    {#if mode === 'review'}
      <Button onclick={cancel}>{msg('common.close')}</Button>
    {:else}
      <!-- The first control, so the default focus: the safe answer. -->
      <Button disabled={saving} reason={saving ? msg('modals.actionRunning') : ''} onclick={cancel}>{msg('common.cancel')}</Button>
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
