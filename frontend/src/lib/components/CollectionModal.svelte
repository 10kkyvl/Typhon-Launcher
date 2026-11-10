<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import Modal from './Modal.svelte';
  import Select from './Select.svelte';
  import TagEditor from './TagEditor.svelte';
  import { GAME_STATUSES, STATUS_LABELS } from '../game/status';
  import { libraryErrorText } from '../library/errors';
  import { genreLabel } from '../metadata/labels';
  import { allTags, deleteCollection, saveCollection, type LibraryCollection } from '../services/library';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    collection = null,
    genres,
  }: { open?: boolean; collection?: LibraryCollection | null; genres: string[] } = $props();

  let name = $state(untrack(() => collection?.name ?? ''));
  let tags = $state<string[]>(untrack(() => [...(collection?.tags ?? [])]));
  let picked = $state<string[]>(untrack(() => [...(collection?.genres ?? [])]));
  let status = $state(untrack(() => collection?.status ?? ''));
  let suggestions = $state<string[]>([]);
  let busy = $state(false);
  let editor: TagEditor;

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

  async function save() {
    editor.commit();
    busy = true;
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
      toast(libraryErrorText(err, msg('modals.collectionSaveFailed')), 'danger');
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!collection) return;
    busy = true;
    try {
      await deleteCollection(collection.id);
      open = false;
    } catch (err) {
      toast(libraryErrorText(err, msg('modals.collectionDeleteFailed')), 'danger');
    } finally {
      busy = false;
    }
  }
</script>

<Modal bind:open title={collection ? msg('modals.collectionEditTitle') : msg('modals.collectionNewTitle')} width="52rem">
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
      <span class="field-label">{msg('modals.collectionGenresLabel')}</span>
      {#if shownGenres.length === 0}
        <p class="hint">{msg('modals.collectionGenresEmpty')}</p>
      {:else}
        <div class="chips">
          {#each shownGenres as genre (genre)}
            <Chip selected={picked.some((own) => fold(own) === fold(genre))} onclick={() => toggleGenre(genre)}>
              {genreLabel(genre)}
            </Chip>
          {/each}
        </div>
      {/if}
    </div>

    <div class="field">
      <span class="field-label">{msg('modals.collectionStatusLabel')}</span>
      <Select options={statusOptions} bind:value={status} />
    </div>
  </div>

  {#snippet footer()}
    {#if collection}
      <span class="remove">
        <Button variant="danger" disabled={busy} onclick={remove}>{msg('common.delete')}</Button>
      </span>
    {/if}
    <Button disabled={busy} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={busy || !name.trim()} onclick={save}>{msg('common.save')}</Button>
  {/snippet}
</Modal>

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

  .hint {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .remove {
    margin-right: auto;
  }
</style>
