<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Select from './Select.svelte';
  import Toggle from './Toggle.svelte';
  import { msg } from '../i18n';
  import {
    consoleForImage,
    consolesForImage,
    imageExtension,
    imageExtensions,
    imageNames,
    narrowByLayout,
    suggestedTitle,
  } from '../library/emulation';
  import { libraryErrorText } from '../library/errors';
  import { imageConsoles, imageTitle, listPlatforms, selectImageFile, type PlatformStatus } from '../services/emulators';
  import { addEmulatedGame, findEmulatedCover, type LibraryGame } from '../services/library';
  import { toast } from '../stores/toasts';

  let {
    open = $bindable(false),
    onadded,
  }: {
    open?: boolean;
    onadded?: (game: LibraryGame) => void;
  } = $props();
  const uid = $props.id();

  let platforms = $state<PlatformStatus[]>([]);
  let loadFailed = $state(false);
  let image = $state('');
  let platform = $state('');
  let title = $state('');
  let findCover = $state(false);
  let adding = $state(false);
  let failure = $state('');
  // fitting is the answer of the disk for the current image; lookup drops a late
  // answer for a path that was typed over in the meantime.
  let fitting = $state<string[]>([]);
  let lookup = 0;
  // suggested is the name last taken from the game itself: it may be replaced by
  // the next one, a name the user typed never is.
  let suggested = '';
  let titleLookup = 0;

  const matching = $derived(narrowByLayout(consolesForImage(platforms, image), fitting));
  const ext = $derived(imageExtension(image));
  const hasGameFiles = $derived(imageNames(platforms).length > 0);
  const noConsole = $derived(Boolean(image) && platforms.length > 0 && matching.length === 0);
  // No console takes the file: said by the image field, which is marked wrong.
  const unmatched = $derived(noConsole && !loadFailed);
  const options = $derived([
    { id: '', label: msg('emul.addConsoleChoose') },
    ...matching.map((item) => ({ id: item.id, label: item.name })),
  ]);
  const ready = $derived(Boolean(image) && Boolean(platform) && Boolean(title.trim()));
  const canAdd = $derived(ready && !adding);

  $effect(() => {
    if (!open) return;
    untrack(() => {
      image = '';
      platform = '';
      title = '';
      suggested = '';
      findCover = false;
      failure = '';
      fitting = [];
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

  $effect(() => {
    const path = image.trim();
    const id = platform;
    if (!open || !path || !id) return;
    const mine = ++titleLookup;
    imageTitle(path, id).then(
      (name) => {
        if (mine !== titleLookup) return;
        const next = suggestedTitle(title, suggested, name);
        if (next === undefined) return;
        title = next;
        suggested = next;
      },
      (err) => console.warn('image title', err),
    );
  });

  async function browse() {
    let path = '';
    try {
      path = await selectImageFile(msg('emul.addImageDialog'), null);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.installedOpenDialogError')), 'danger');
      return;
    }
    if (path) pick(path);
  }

  function pick(path: string) {
    image = path;
    failure = '';
    fitting = [];
    platform = consoleForImage(platforms, path);
    const mine = ++lookup;
    if (consolesForImage(platforms, path).length < 2) return;
    imageConsoles(path).then(
      (ids) => {
        if (mine !== lookup) return;
        fitting = ids;
        const narrowed = narrowByLayout(consolesForImage(platforms, path), ids);
        if (!narrowed.some((item) => item.id === platform)) {
          platform = narrowed.length === 1 ? narrowed[0].id : '';
        }
      },
      (err) => console.warn('image consoles', err),
    );
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
    <div class="field">
      <label class="field-label" for="{uid}-image">{msg('emul.addImageLabel')}</label>
      <div class="field-row">
        <input
          id="{uid}-image"
          class="input"
          type="text"
          placeholder={msg('emul.addImagePlaceholder')}
          aria-invalid={unmatched ? 'true' : undefined}
          aria-describedby={[unmatched ? `${uid}-image-error` : '', hasGameFiles ? `${uid}-image-hint` : ''].filter(Boolean).join(' ') || undefined}
          value={image}
          oninput={(e) => pick(e.currentTarget.value)}
        />
        <Button onclick={browse}>{msg('games.installedBrowseButton')}</Button>
      </div>
      {#if hasGameFiles}<span class="form-hint" id="{uid}-image-hint">{msg('emul.addGameFileHint')}</span>{/if}
      {#if unmatched}
        <p class="failure" id="{uid}-image-error" role="alert">
          {msg('emul.addNoConsole', { ext, list: [...imageExtensions(platforms), ...imageNames(platforms)].join(' ') })}
        </p>
      {/if}
    </div>
    {#if loadFailed}
      <Notice tone="danger" compact>{msg('emul.addConsolesFailed')}</Notice>
    {:else if !noConsole && image}
      <div class="field">
        <span class="field-label" id="{uid}-console-label">{msg('emul.addConsoleLabel')}</span>
        <Select bind:value={platform} {options} width="100%" labelledby="{uid}-console-label" />
        {#if matching.length > 1}
          <p class="form-hint">{msg('emul.addConsoleAmbiguous')}</p>
        {/if}
      </div>
    {/if}
    <div class="field">
      <label class="field-label" for="{uid}-title">{msg('games.installedTitleFieldLabel')}</label>
      <input
        id="{uid}-title"
        class="input"
        type="text"
        placeholder={msg('games.installedTitlePlaceholder')}
        aria-describedby="{uid}-title-hint"
        bind:value={title}
      />
      <span class="form-hint" id="{uid}-title-hint">{msg('emul.addTitleHint')}</span>
    </div>
    <div class="toggle-row">
      <div class="field">
        <span class="field-label" id="{uid}-add-find-cover-label">{msg('emul.addFindCover')}</span>
        <span class="form-hint" id="{uid}-add-find-cover-sub">{msg('emul.addFindCoverHint')}</span>
      </div>
      <Toggle bind:checked={findCover} labelledby="{uid}-add-find-cover-label" describedby="{uid}-add-find-cover-sub" />
    </div>
    {#if failure}<Notice tone="danger" compact>{failure}</Notice>{/if}
  </div>
  {#snippet footer()}
    <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={!ready} busy={adding} onclick={submit}>
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
