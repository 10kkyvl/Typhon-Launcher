<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import TagEditor from './TagEditor.svelte';
  import { libraryErrorText } from '../library/errors';
  import { allTags, setTags, type LibraryGame } from '../services/library';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let { open = $bindable(false), game }: { open?: boolean; game: LibraryGame } = $props();

  let tags = $state<string[]>(untrack(() => [...(game.tags ?? [])]));
  let suggestions = $state<string[]>([]);
  let saving = $state(false);
  let editor: TagEditor;

  $effect(() => {
    let current = true;
    allTags().then(
      (list) => {
        if (current) suggestions = list;
      },
      (err) => toast(libraryErrorText(err, msg('modals.tagSuggestionsFailed')), 'danger'),
    );
    return () => {
      current = false;
    };
  });

  async function save() {
    editor.commit();
    saving = true;
    try {
      await setTags(game.id, tags);
      open = false;
    } catch (err) {
      toast(libraryErrorText(err, msg('modals.gameTagsSaveFailed')), 'danger');
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:open title={msg('modals.gameTagsTitle')}>
  <TagEditor bind:this={editor} bind:tags {suggestions} placeholder={msg('modals.gameTagsPlaceholder')} disabled={saving} />
  {#if tags.length === 0}
    <p class="empty">{msg('modals.gameTagsEmpty')}</p>
  {/if}
  {#snippet footer()}
    <Button disabled={saving} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={saving} onclick={save}>{msg('common.save')}</Button>
  {/snippet}
</Modal>

<style>
  .empty {
    margin: var(--space-3) 0 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
