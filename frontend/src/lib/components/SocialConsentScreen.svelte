<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { loadFriends, saveSocialConsent } from '../stores/social';
  import { toast } from '../stores/toasts';
  import { focusHandOff } from '../utils/focusHandOff';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { errorCode, hasMessage, msg } from '../i18n';

  /**
   * The consent to sync the library with the account, which the friends, the feed and the players'
   * pages need. Giving it is a legal act: it is sent once, only by Enable's own press, and "Not now"
   * is the first focus. The consent shows in the settings only once Go has saved it, so the screen
   * under the dialog reads nothing before that (the store's optimistic update would show it at once,
   * while Go still refuses every social call); then the dialog reads the friends and closes. A failed
   * save is said here and the dialog stays. An answer for an opening that is over, or for a dialog
   * that is gone, is said once in a toast.
   */
  let { open = $bindable(false), onclose }: { open?: boolean; onclose?: () => void } = $props();
  const uid = $props.id();
  const failureId = `${uid}-failure`;

  let saving = $state(false);
  let failure = $state('');
  let opening = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });
  $effect(() => {
    void open;
    untrack(() => {
      opening += 1;
      saving = false;
      failure = '';
    });
  });

  function close() {
    open = false;
    onclose?.();
  }

  function failureText(err: unknown): string {
    const code = errorCode(err);
    return hasMessage(code) ? msg('modals.socialConsentEnableFailedReason', { reason: msg(code) }) : msg('modals.socialConsentEnableFailed');
  }

  async function enable() {
    if (saving) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    saving = true;
    failure = '';
    try {
      await saveSocialConsent();
    } catch (err) {
      if (heard()) {
        failure = failureText(err);
        saving = false;
        await hand(() => document.getElementById(failureId));
      } else {
        toast(failureText(err), 'danger');
      }
      return;
    }
    if (!heard()) {
      toast(msg('modals.socialConsentEnabled'), 'success');
      return;
    }
    await loadFriends();
    if (!heard()) return;
    saving = false;
    close();
  }

  const heldReason = $derived(saving ? msg('modals.actionRunning') : '');
</script>

<Modal dismissible={!saving} busy={saving} bind:open title={msg('modals.socialConsentTitle')} width="50rem" onclose={() => onclose?.()}>
  <div class="body">
    <p class="text">
      {msg('modals.socialConsentIntro')}
    </p>
    <p class="text">{msg('modals.socialConsentListIntro')}</p>
    <ul class="list">
      <li>{msg('modals.socialConsentListGames')}</li>
      <li>{msg('modals.socialConsentListPlaytime')}</li>
      <li>{msg('modals.socialConsentListFavoritesStatus')}</li>
      <li>{msg('modals.socialConsentListDates')}</li>
    </ul>
    <p class="text">
      {msg('modals.socialConsentOutro')}
    </p>
    {#if failure}<Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice>{/if}
  </div>

  {#snippet footer()}
    <span class="safe" data-autofocus>
      <Button disabled={saving} reason={heldReason} onclick={close}>{msg('modals.socialConsentNotNow')}</Button>
    </span>
    <Button variant="primary" busy={saving} onclick={enable}>
      {saving ? msg('modals.socialConsentEnabling') : msg('modals.socialConsentEnable')}
    </Button>
  {/snippet}
</Modal>

<style>
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .text {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding-left: 2rem;
    list-style: disc;
    font-size: var(--font-sm);
    line-height: 1.55;
    color: var(--text-2);
  }

  .safe {
    display: contents;
  }
</style>
