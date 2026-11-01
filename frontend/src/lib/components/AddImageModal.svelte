<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Select from './Select.svelte';
  import Toggle from './Toggle.svelte';
  import { msg } from '../i18n';
  import {
    consoleForImage,
    consolesForImage,
    imageExtension,
    imageExtensions,
    titleFromImage,
  } from '../library/emulation';
  import { libraryErrorText } from '../library/errors';
  import { listPlatforms, selectImageFile, type PlatformStatus } from '../services/emulators';
  import { addEmulatedGame, findEmulatedCover, type LibraryGame } from '../services/library';
  import { toast } from '../stores/toasts';

  let {
    open = $bindable(false),
    onadded,
  }: {
    open?: boolean;
    onadded?: (game: LibraryGame) => void;
  } = $props();

  let platforms = $state<PlatformStatus[]>([]);
  let loadFailed = $state(false);
  let image = $state('');
  let platform = $state('');
  let title = $state('');
  let titleTouched = $state(false);
  let findCover = $state(false);
  let adding = $state(false);
  let failure = $state('');

  const matching = $derived(consolesForImage(platforms, image));
  const ext = $derived(imageExtension(image));
  const noConsole = $derived(Boolean(image) && platforms.length > 0 && matching.length === 0);
  const options = $derived([
    { id: '', label: msg('emul.addConsoleChoose') },
    ...matching.map((item) => ({ id: item.id, label: item.name })),
  ]);
  const canAdd = $derived(Boolean(image) && Boolean(platform) && Boolean(title.trim()) && !adding);

  $effect(() => {
    if (!open) return;
    untrack(() => {
      image = '';
      platform = '';
      title = '';
      titleTouched = false;
      findCover = false;
      failure = '';
      loadFailed = false;
    });
    let stale = false;
    listPlatforms().then(
      (list) => {
        if (!stale) platforms = list;
      },
      (err) => {
        console.error('list consoles', err);
        if (!stale) loadFailed = true;
      },
    );
    return () => {
      stale = true;
    };
  });

  async function browse() {
    let path = '';
    try {
      path = await selectImageFile(msg('emul.addImageDialog'));
    } catch (err) {
      toast(libraryErrorText(err, msg('games.installedOpenDialogError')), 'danger');
      return;
    }
    if (path) pick(path);
  }

  function pick(path: string) {
    image = path;
    failure = '';
    platform = consoleForImage(platforms, path);
    if (!titleTouched) title = titleFromImage(path);
  }

  async function lookUpCover(game: LibraryGame) {
    try {
      await findEmulatedCover(game.id);
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.coverFailed')));
    }
  }

  async function submit() {
    if (!canAdd) return;
    adding = true;
    failure = '';
    try {
      const game = await addEmulatedGame(image.trim(), platform, title.trim());
      toast(msg('games.installedGameAddedToast', { title: game.title }), 'success');
      open = false;
      onadded?.(game);
      if (findCover) await lookUpCover(game);
    } catch (err) {
      failure = libraryErrorText(err, msg('emul.addFailed'));
    } finally {
      adding = false;
    }
  }
</script>

<Modal bind:open title={msg('emul.addTitle')}>
  <div class="form">
    <label class="field">
      <span class="field-label">{msg('emul.addImageLabel')}</span>
      <div class="field-row">
        <input
          class="input"
          type="text"
          placeholder={msg('emul.addImagePlaceholder')}
          value={image}
          oninput={(e) => pick(e.currentTarget.value)}
        />
        <Button onclick={browse}>{msg('games.installedBrowseButton')}</Button>
      </div>
    </label>
    {#if loadFailed}
      <p class="failure" role="alert">{msg('emul.addConsolesFailed')}</p>
    {:else if noConsole}
      <p class="failure" role="alert">
        {msg('emul.addNoConsole', { ext, list: imageExtensions(platforms).join(' ') })}
      </p>
    {:else if image}
      <div class="field">
        <span class="field-label">{msg('emul.addConsoleLabel')}</span>
        <Select bind:value={platform} {options} width="100%" />
        {#if matching.length > 1}
          <p class="form-hint">{msg('emul.addConsoleAmbiguous')}</p>
        {/if}
      </div>
    {/if}
    <label class="field">
      <span class="field-label">{msg('games.installedTitleFieldLabel')}</span>
      <input
        class="input"
        type="text"
        placeholder={msg('games.installedTitlePlaceholder')}
        bind:value={title}
        oninput={() => (titleTouched = true)}
      />
      <span class="form-hint">{msg('emul.addTitleHint')}</span>
    </label>
    <div class="toggle-row">
      <div class="field">
        <span class="field-label">{msg('emul.addFindCover')}</span>
        <span class="form-hint">{msg('emul.addFindCoverHint')}</span>
      </div>
      <Toggle bind:checked={findCover} label={msg('emul.addFindCover')} />
    </div>
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  </div>
  {#snippet footer()}
    <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={!canAdd} onclick={submit}>
      {adding ? msg('games.installedAddingLabel') : msg('common.add')}
    </Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .field-label {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
  }

  .toggle-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .field-row {
    display: flex;
    gap: 0.8rem;
  }

  .form-hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .failure {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--danger);
  }
</style>
