<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import EmptyState from './EmptyState.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Select from './Select.svelte';
  import TagEditor from './TagEditor.svelte';
  import { GAME_STATUSES, STATUS_LABELS } from '../game/status';
  import { libraryErrorText } from '../library/errors';
  import { genreLabel } from '../metadata/labels';
  import type { ConfirmPrompt } from '../confirm/prompts';
  import { allTags, deleteCollection, saveCollection, type LibraryCollection } from '../services/library';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    collection = null,
    genres,
    returnFocus,
  }: {
    open?: boolean;
    collection?: LibraryCollection | null;
    genres: string[];
    /** Where focus goes on close when the element that opened the editor is gone (it left with a deleted collection). */
    returnFocus?: () => HTMLElement | null | undefined;
  } = $props();
  const uid = $props.id();

  let name = $state(untrack(() => collection?.name ?? ''));
  let tags = $state<string[]>(untrack(() => [...(collection?.tags ?? [])]));
  let picked = $state<string[]>(untrack(() => [...(collection?.genres ?? [])]));
  let status = $state(untrack(() => collection?.status ?? ''));
  let suggestions = $state<string[]>([]);
  let busy = $state(false);
  // Which footer button set `busy`: that one stays focused and busy, the others wait.
  let pressed = $state<'' | 'save' | 'remove'>('');
  let editor: TagEditor;
  let confirming = $state(false);
  // Why the last save or delete was refused, in the launcher's words; cleared by the next try.
  let failure = $state('');
  let noCriteria = $state(false);

  const fold = (value: string) => value.normalize('NFKC').toLocaleLowerCase();
  const shownGenres = $derived(
    [...new Map([...genres, ...picked].map((genre) => [fold(genre), genre])).values()].toSorted((a, b) =>
      genreLabel(a).localeCompare(genreLabel(b)),
    ),
  );
  const statusOptions = $derived([
    { id: '', label: msg('modals.collectionStatusAny') },
    ...GAME_STATUSES.map((value) => ({ id: value, label: STATUS_LABELS[value] })),
  ]);

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

  function toggleGenre(genre: string) {
    picked = picked.some((own) => fold(own) === fold(genre))
      ? picked.filter((own) => fold(own) !== fold(genre))
      : [...picked, genre];
  }

  $effect(() => {
    if (tags.length > 0 || picked.length > 0 || status) noCriteria = false;
  });

  async function save() {
    editor.commit();
    failure = '';
    // Without a criterion the collection would match every game (lib/library/collections.ts).
    if (tags.length === 0 && picked.length === 0 && !status) {
      noCriteria = true;
      return;
    }
    busy = true;
    pressed = 'save';
    try {
      await saveCollection({
        id: collection?.id ?? '',
        name: name.trim(),
        tags,
        genres: picked,
        status,
        createdAt: collection?.createdAt ?? '',
        updatedAt: collection?.updatedAt ?? '',
      });
      open = false;
    } catch (err) {
      failure = libraryErrorText(err, msg('modals.collectionSaveFailed'));
    } finally {
      busy = false;
      pressed = '';
    }
  }

  const deletePrompt = $derived<ConfirmPrompt>({
    title: msg('library.collectionDeleteTitle'),
    text: msg('library.collectionDeleteText', { name: collection?.name ?? '' }),
    confirm: msg('common.delete'),
  });

  // The confirmation only asks: the delete runs here, so the editor's Delete stays busy and focused
  // and a failure is said in the editor.
  function confirmRemove() {
    confirming = false;
    void remove();
  }

  async function remove() {
    if (!collection) return;
    failure = '';
    busy = true;
    pressed = 'remove';
    try {
      await deleteCollection(collection.id);
      open = false;
    } catch (err) {
      failure = libraryErrorText(err, msg('modals.collectionDeleteFailed'));
    } finally {
      busy = false;
      pressed = '';
    }
  }
</script>

<Modal dismissible={!busy} bind:open title={collection ? msg('modals.collectionEditTitle') : msg('modals.collectionNewTitle')} width="52rem" {returnFocus}>
  <div class="form">
    <label class="field">
      <span class="field-label">{msg('modals.collectionNameLabel')}</span>
      <input
        class="input"
        type="text"
        maxlength="48"
        placeholder={msg('modals.collectionNamePlaceholder')}
        disabled={busy}
        bind:value={name}
      />
    </label>

    <div class="field">
      <span class="field-label">{msg('modals.collectionTagsLabel')}</span>
      <TagEditor bind:this={editor} bind:tags {suggestions} placeholder={msg('modals.gameTagsPlaceholder')} disabled={busy} />
    </div>

    <div class="field">
      <span class="field-label" id="{uid}-genres-label">{msg('modals.collectionGenresLabel')}</span>
      {#if shownGenres.length === 0}
        <EmptyState size="inline" title={msg('modals.collectionGenresEmpty')} />
      {:else}
        <div class="chips" role="group" aria-labelledby="{uid}-genres-label">
          {#each shownGenres as genre (genre)}
            <Chip selected={picked.some((own) => fold(own) === fold(genre))} onclick={() => toggleGenre(genre)}>
              {genreLabel(genre)}
            </Chip>
          {/each}
        </div>
      {/if}
    </div>

    <div class="field">
      <span class="field-label" id="{uid}-status-label">{msg('modals.collectionStatusLabel')}</span>
      <Select options={statusOptions} bind:value={status} labelledby="{uid}-status-label" />
    </div>

    <div role="alert">
      {#if noCriteria}<span class="error">{msg('library.collectionNoCriteria')}</span>{/if}
    </div>

    {#if failure}
      <Notice tone="danger" compact>{failure}</Notice>
    {/if}
  </div>

  {#snippet footer()}
    {#if collection}
      <span class="remove">
        <Button variant="danger" disabled={busy && pressed !== 'remove'} busy={pressed === 'remove'} onclick={() => (confirming = true)}>{msg('common.delete')}</Button>
      </span>
    {/if}
    <Button disabled={busy} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={(busy && pressed !== 'save') || !name.trim()} busy={pressed === 'save'} onclick={save}>{msg('common.save')}</Button>
  {/snippet}
</Modal>

{#if confirming}
  <ConfirmModal
    prompt={deletePrompt}
    onconfirm={confirmRemove}
    onclose={() => (confirming = false)}
    errorText={(err) => libraryErrorText(err, msg('modals.collectionDeleteFailed'))}
  />
{/if}

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .field-label {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
    max-height: 18rem;
    overflow-y: auto;
  }

  .remove {
    margin-right: auto;
  }

  .error {
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--danger);
  }
</style>
