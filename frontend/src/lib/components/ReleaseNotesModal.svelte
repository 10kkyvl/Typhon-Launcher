<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ReleaseNotesList from './ReleaseNotesList.svelte';
  import { dismissReleaseNotes, releaseNotes, unseenReleaseNotes } from '../stores/selfupdate';
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
  width="52rem"
  onclose={dismissReleaseNotes}
>
  {#if notes.length > 1}
    <p class="lead">{msg('modals.releaseNotesMultiple')}</p>
  {/if}
  <ReleaseNotesList notes={notes} currentVersion={$releaseNotes.currentVersion} />
  {#snippet footer()}
    <Button variant="primary" onclick={dismissReleaseNotes}>{msg('common.gotIt')}</Button>
  {/snippet}
</Modal>

<style>
  .lead {
    margin: 0 0 var(--space-4);
    color: var(--text-2);
    font-size: var(--font-sm);
  }
</style>
