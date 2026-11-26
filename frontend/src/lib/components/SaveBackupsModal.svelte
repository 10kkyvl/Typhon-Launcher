<script lang="ts">
  import Button from './Button.svelte';
  import GameSaves from './GameSaves.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import { closeSaveBackups, saveBackupTarget, savesInFlight } from '../stores/savebackup';

  const target = $derived($saveBackupTarget);
  let busy = $state(false);
  // A backup action runs in the store and outlives the dialog (opened again, it shows it busy), so the
  // dialog may close meanwhile, by every way at once. A folder action is the view's own and its answer
  // is said here: the dialog waits for it, Close says why.
  const backupRuns = $derived(target ? Boolean($savesInFlight.get(target.gameId)?.backup) : false);
  const held = $derived(busy && !backupRuns);
</script>

<Modal
  dismissible={!held}
  open={!!target}
  title={target ? msg('saves.title', { title: target.title }) : msg('games.actionSavesBackups')}
  width="64rem"
  onclose={closeSaveBackups}
>
  {#if target}
    <GameSaves gameId={target.gameId} backups bind:busy />
  {/if}

  {#snippet footer()}
    <Button disabled={held} reason={held ? msg('modals.actionRunning') : ''} onclick={closeSaveBackups}>{msg('common.close')}</Button>
  {/snippet}
</Modal>
