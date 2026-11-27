<script lang="ts">
  import AvatarCropModal from './AvatarCropModal.svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import { removeAvatarPrompt } from '../confirm/prompts';
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
  import type { CropRect } from '../utils/crop';
  import { msg } from '../i18n';

  let {
    size = 'md',
    disabled = false,
    reason,
  }: {
    size?: 'md' | 'sm';
    disabled?: boolean;
    /**
     * Why the buttons are refused while `disabled`: they then stay in the Tab order and say it, as a
     * Button with a reason does. Pass '' while there is none. Without it, `disabled` disables them.
     */
    reason?: string;
  } = $props();

  let cropOpen = $state(false);
  let cropSrc = $state('');
  let failure = $state('');
  let confirmingRemove = $state(false);

  // The picker is "Change avatar"'s own work: it stays busy under the focus it was pressed with. An
  // upload or a removal runs in a dialog of its own, which holds the focus meanwhile.
  const working = $derived($uploadingAvatar || $removingAvatar);

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

  async function save(encoded: string, crop: CropRect) {
    failure = '';
    try {
      await saveAvatar(encoded, crop);
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
    <Button {size} busy={$pickingAvatar} disabled={working || disabled} reason={reason === undefined ? undefined : disabled ? reason : ''} onclick={pick}>
      {$pickingAvatar ? msg('ui.avatarPicking') : msg('ui.avatarChange')}
    </Button>
    <!-- With no avatar there is nothing to remove: it stays plainly disabled. -->
    <Button
      {size}
      variant="danger"
      disabled={$pickingAvatar || working || disabled || !$currentUser?.avatarUrl}
      reason={reason === undefined ? undefined : disabled && $currentUser?.avatarUrl ? reason : ''}
      onclick={() => (confirmingRemove = true)}
    >
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

{#if confirmingRemove}
  <ConfirmModal prompt={removeAvatarPrompt()} onconfirm={remove} onclose={() => (confirmingRemove = false)} />
{/if}

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
