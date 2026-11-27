<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ReleaseNotesList from './ReleaseNotesList.svelte';
  import RollbackAction from './RollbackAction.svelte';
  import { dismissReleaseNotes, releaseNotes, rollbackInfo, unseenReleaseNotes } from '../stores/selfupdate';
  import { updateShow } from '../stores/updateShow';
  import { msg } from '../i18n';

  const notes = $derived($unseenReleaseNotes);
  // Not under the update show: a dialog would make the show inert and take focus behind its curtain.
  const open = $derived(notes.length > 0 && !$updateShow);
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
  <!-- A stop in the Tab order, and the first one, so the keys scroll the notes from the opening; the
       versions' headings come under the dialog's title. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="notes" role="document" aria-label={title} tabindex="0">
    <ReleaseNotesList notes={notes} currentVersion={$releaseNotes.currentVersion} />
  </div>
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

  .notes {
    border-radius: var(--radius-xs);
  }

  .lead {
    margin: 0 0 var(--space-4);
    color: var(--text-2);
    font-size: var(--font-sm);
  }
</style>
