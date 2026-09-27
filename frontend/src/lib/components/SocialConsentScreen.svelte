<script lang="ts">
  import { get } from 'svelte/store';
  import { accountErrorText } from '../services/accountMessages';
  import { settings, updateSettings } from '../stores/settings';
  import { loadFriends } from '../stores/social';
  import { toast } from '../stores/toasts';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';

  let { open = $bindable(false), onclose }: { open?: boolean; onclose?: () => void } = $props();

  let saving = $state(false);

  function close() {
    open = false;
    onclose?.();
  }

  async function enable() {
    if (saving) return;
    saving = true;
    try {
      await updateSettings({ accountSync: true });
      if (!get(settings)?.accountSync) return;
      await loadFriends();
      close();
    } catch (err) {
      toast(accountErrorText(err, msg('modals.socialConsentEnableFailed')), 'danger');
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:open title={msg('modals.socialConsentTitle')} width="50rem" onclose={() => onclose?.()}>
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
  </div>

  {#snippet footer()}
    <Button disabled={saving} onclick={close}>{msg('modals.socialConsentNotNow')}</Button>
    <Button variant="primary" disabled={saving} onclick={enable}>
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
</style>
