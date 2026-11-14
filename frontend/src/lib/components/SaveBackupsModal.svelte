<script lang="ts">
  import Button from './Button.svelte';
  import GameSaves from './GameSaves.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import { closeSaveBackups, saveBackupTarget } from '../stores/savebackup';

  const target = $derived($saveBackupTarget);
  let busy = $state(false);
</script>

<Modal
  dismissible={!busy}
  open={!!target}
  title={target ? msg('saves.title', { title: target.title }) : msg('games.actionSavesBackups')}
  width="64rem"
  onclose={closeSaveBackups}
>
  {#if target}
    <GameSaves gameId={target.gameId} bind:busy />
  {/if}

  {#snippet footer()}
    <Button onclick={closeSaveBackups}>{msg('common.close')}</Button>
  {/snippet}
</Modal>
