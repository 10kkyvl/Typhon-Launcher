<script lang="ts">
  import AvatarCropModal from './AvatarCropModal.svelte';
  import Button from './Button.svelte';
  import { accountErrorText } from '../services/accountMessages';
  import {
    chooseAvatar,
    currentUser,
    deleteAvatar,
    pickingAvatar,
    removingAvatar,
    saveAvatar,
    uploadingAvatar,
  } from '../stores/user';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let { size = 'md', disabled = false }: { size?: 'md' | 'sm'; disabled?: boolean } = $props();

  let cropOpen = $state(false);
  let cropSrc = $state('');
  let failure = $state('');

  const busy = $derived($pickingAvatar || $uploadingAvatar || $removingAvatar);

  async function pick() {
    failure = '';
    try {
      const src = await chooseAvatar();
      if (!src) return;
      cropSrc = src;
      cropOpen = true;
    } catch (err) {
      failure = accountErrorText(err, msg('ui.avatarOpenFailed'));
    }
  }

  async function save(encoded: string) {
    failure = '';
    try {
      await saveAvatar(encoded);
      cropOpen = false;
      cropSrc = '';
      toast(msg('ui.avatarUpdated'), 'success');
    } catch (err) {
      failure = accountErrorText(err, msg('ui.avatarUpdateFailed'));
    }
  }

  async function remove() {
    failure = '';
    try {
      await deleteAvatar();
      toast(msg('ui.avatarDeleted'), 'success');
    } catch (err) {
      failure = accountErrorText(err, msg('ui.avatarDeleteFailed'));
    }
  }
</script>

<div class="avatar-editor">
  <div class="buttons">
    <Button {size} disabled={busy || disabled} onclick={pick}>
      {$pickingAvatar ? msg('ui.avatarPicking') : msg('ui.avatarChange')}
    </Button>
    <Button {size} variant="danger" disabled={busy || disabled || !$currentUser?.avatarUrl} onclick={remove}>
      {$removingAvatar ? msg('ui.avatarRemoving') : msg('ui.avatarRemove')}
    </Button>
  </div>
  {#if failure && !cropOpen}<span class="error">{failure}</span>{/if}
</div>

<AvatarCropModal
  bind:open={cropOpen}
  src={cropSrc}
  saving={$uploadingAvatar}
  error={cropOpen ? failure : ''}
  onsave={save}
/>

<style>
  .avatar-editor {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
  }

  .buttons {
    display: flex;
    gap: 0.8rem;
    flex-wrap: wrap;
  }

  .error {
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
