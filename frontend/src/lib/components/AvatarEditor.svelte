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
      failure = accountErrorText(err, 'Не удалось открыть изображение');
    }
  }

  async function save(encoded: string) {
    failure = '';
    try {
      await saveAvatar(encoded);
      cropOpen = false;
      cropSrc = '';
      toast('Аватар обновлён', 'success');
    } catch (err) {
      failure = accountErrorText(err, 'Не удалось обновить аватар');
    }
  }

  async function remove() {
    failure = '';
    try {
      await deleteAvatar();
      toast('Аватар удалён', 'success');
    } catch (err) {
      failure = accountErrorText(err, 'Не удалось удалить аватар');
    }
  }
</script>

<div class="avatar-editor">
  <div class="buttons">
    <Button {size} disabled={busy || disabled} onclick={pick}>
      {$pickingAvatar ? 'Выбор файла…' : 'Сменить аватар'}
    </Button>
    <Button {size} variant="danger" disabled={busy || disabled || !$currentUser?.avatarUrl} onclick={remove}>
      {$removingAvatar ? 'Удаление…' : 'Удалить аватар'}
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
