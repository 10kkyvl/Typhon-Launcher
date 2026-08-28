<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ReleaseNotesList from './ReleaseNotesList.svelte';
  import { dismissReleaseNotes, releaseNotes, unseenReleaseNotes } from '../stores/selfupdate';

  const notes = $derived($unseenReleaseNotes);
  const open = $derived(notes.length > 0);
  const title = $derived(
    notes.length === 1 ? `Что нового в версии ${notes[0].version}` : `Что нового в версии ${$releaseNotes.currentVersion}`,
  );
</script>

<Modal
  open={open}
  {title}
  width="52rem"
  onclose={dismissReleaseNotes}
>
  {#if notes.length > 1}
    <p class="lead">Лаунчер обновился сразу на несколько версий — вот что изменилось.</p>
  {/if}
  <ReleaseNotesList notes={notes} currentVersion={$releaseNotes.currentVersion} />
  {#snippet footer()}
    <Button variant="primary" onclick={dismissReleaseNotes}>Понятно</Button>
  {/snippet}
</Modal>

<style>
  .lead {
    margin: 0 0 var(--space-4);
    color: var(--text-2);
    font-size: var(--font-sm);
  }
</style>
