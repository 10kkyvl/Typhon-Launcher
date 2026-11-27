<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import TagEditor from './TagEditor.svelte';
  import { libraryErrorText } from '../library/errors';
  import { allTags, setTags, type LibraryGame } from '../services/library';
  import { focusHandOff } from '../utils/focusHandOff';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    game,
    returnFocus,
  }: {
    open?: boolean;
    game: LibraryGame;
    /** Where focus goes on close when the element that opened the dialog is gone (a menu item). */
    returnFocus?: () => HTMLElement | null | undefined;
  } = $props();
  const uid = $props.id();
  const failureId = `${uid}-failure`;

  let tags = $state<string[]>(untrack(() => [...(game.tags ?? [])]));
  let suggestions = $state<string[]>([]);
  let saving = $state(false);
  let editor: TagEditor;
  // Why the save was refused, and why the library's tags could not be read, in the launcher's words.
  let failure = $state('');
  let suggestionsFailure = $state('');
  let retrying = $state(false);

  // The dialog is mounted for one opening: an answer that comes after it closed says nothing.
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  async function readSuggestions(): Promise<boolean | undefined> {
    try {
      const list = await allTags();
      if (!alive) return undefined;
      suggestions = list;
      suggestionsFailure = '';
      return true;
    } catch (err) {
      if (!alive) return undefined;
      suggestionsFailure = libraryErrorText(err, msg('modals.tagSuggestionsFailed'));
      return false;
    }
  }

  onMount(() => {
    void readSuggestions();
  });

  async function save() {
    if (saving) return;
    editor.commit();
    // The focus Save holds at the press goes to the words of a refusal.
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    failure = '';
    saving = true;
    try {
      await setTags(game.id, tags);
      // A dialog unmounted meanwhile (the page turned to another game) closes nothing of the one now open.
      if (alive) open = false;
    } catch (err) {
      if (alive) failure = libraryErrorText(err, msg('modals.gameTagsSaveFailed'));
    } finally {
      saving = false;
    }
    if (failure) await hand(() => document.getElementById(failureId));
  }
</script>

<Modal dismissible={!saving} bind:open title={msg('modals.gameTagsTitle')} {returnFocus}>
  <TagEditor bind:this={editor} bind:tags {suggestions} placeholder={msg('modals.gameTagsPlaceholder')} disabled={saving} />
  {#if tags.length === 0}
    <div class="no-tags">
      <EmptyState size="inline" title={msg('modals.gameTagsEmpty')} />
    </div>
  {/if}
  {#if suggestionsFailure || retrying}
    <div class="notice">
      <Notice tone="danger" compact bind:retrying retry={{ label: msg('common.retry'), run: readSuggestions }}>{suggestionsFailure}</Notice>
    </div>
  {/if}
  {#if failure}
    <div class="notice">
      <Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice>
    </div>
  {/if}
  {#snippet footer()}
    <Button disabled={saving} reason={saving ? msg('modals.actionRunning') : ''} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" busy={saving} onclick={save}>{msg('common.save')}</Button>
  {/snippet}
</Modal>

<style>
  .no-tags,
  .notice {
    margin-top: var(--space-3);
  }
</style>
