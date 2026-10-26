<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ReleaseNotesList from './ReleaseNotesList.svelte';
  import RollbackAction from './RollbackAction.svelte';
  import { dismissReleaseNotes, releaseNotes, rollbackInfo, unseenReleaseNotes } from '../stores/selfupdate';
  import { msg } from '../i18n';

  const notes = $derived($unseenReleaseNotes);
  const open = $derived(notes.length > 0);
  const title = $derived(
    msg('modals.releaseNotesTitle', { version: notes.length === 1 ? notes[0].version : $releaseNotes.currentVersion }),
  );
</script>

<Modal
  open={open}
  {title}
  width="104rem"
  onclose={dismissReleaseNotes}
>
  {#if notes.length > 1}
    <p class="lead">{msg('modals.releaseNotesMultiple')}</p>
  {/if}
  <ReleaseNotesList notes={notes} currentVersion={$releaseNotes.currentVersion} />
  {#snippet footer()}
    {#if $rollbackInfo.available}
      <span class="rollback"><RollbackAction compact /></span>
    {/if}
    <Button variant="primary" onclick={dismissReleaseNotes}>{msg('common.gotIt')}</Button>
  {/snippet}
</Modal>

<style>
  .rollback {
    margin-right: auto;
  }

  .lead {
    margin: 0 0 var(--space-4);
    color: var(--text-2);
    font-size: var(--font-sm);
  }
</style>
