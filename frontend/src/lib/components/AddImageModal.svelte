<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import PathField from './PathField.svelte';
  import Select from './Select.svelte';
  import TextField from './TextField.svelte';
  import Toggle from './Toggle.svelte';
  import { errorCode, hasMessage, msg } from '../i18n';
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
  // A refused add, said where it belongs: at the image or the title it concerns, else in the notice.
  let failure = $state('');
  let imageRefused = $state('');
  let titleRefused = $state('');
  let imageInput = $state<HTMLInputElement>();
  let titleInput = $state<HTMLInputElement>();
  // The path the console lookups last ran for: typing and Browse both go through pick().
  let picked = '';
  // A dialog gone with its screen hears nothing: Go's answer is said once in a toast naming the game.
  let alive = true;
  onDestroy(() => {
    alive = false;
  });
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
      imageRefused = '';
      titleRefused = '';
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
    failure = '';
    try {
      path = await selectImageFile(msg('emul.addImageDialog'), null);
    } catch (err) {
      if (alive) failure = libraryErrorText(err, msg('games.installedOpenDialogError'));
      return;
    }
    if (path && alive) pick(path);
  }

  $effect(() => {
    const path = image;
    untrack(() => {
      if (path !== picked) pick(path);
    });
  });

  function pick(path: string) {
    picked = path;
    image = path;
    failure = '';
    imageRefused = '';
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

  // The dialog has closed by then: a failed look-up is said in a toast naming the game.
  async function lookUpCover(game: LibraryGame) {
    try {
      await findEmulatedCover(game.id);
    } catch (err) {
      const code = errorCode(err);
      const title = game.title;
      toast(hasMessage(code) ? msg('emul.coverFailedForReason', { title, reason: msg(code) }) : msg('emul.coverFailedFor', { title }));
    }
  }

  // Go's refusals of a path or a file, said at the image field; an empty title at the title field.
  const IMAGE_CODES = /^emulators\.(image_|pkg_invalid|sfo_|title_id_invalid|vpk_invalid)/;

  async function submit() {
    if (!canAdd) return;
    const name = title.trim();
    const cover = findCover;
    adding = true;
    failure = '';
    imageRefused = '';
    titleRefused = '';
    let game: LibraryGame;
    try {
      game = await addEmulatedGame(image.trim(), platform, name);
    } catch (err) {
      const code = errorCode(err);
      if (!alive) {
        toast(hasMessage(code) ? msg('emul.addFailedForReason', { title: name, reason: msg(code) }) : msg('emul.addFailedFor', { title: name }), 'danger');
        return;
      }
      adding = false;
      const text = libraryErrorText(err, msg('emul.addFailed'));
      if (IMAGE_CODES.test(code)) {
        imageRefused = text;
        await tick();
        imageInput?.focus();
      } else if (code === 'emulators.title_empty') {
        titleRefused = text;
        await tick();
        titleInput?.focus();
      } else {
        failure = text;
      }
      return;
    }
    toast(msg('games.installedGameAddedToast', { title: game.title }), 'success');
    if (alive) {
      adding = false;
      open = false;
      onadded?.(game);
    }
    if (cover) await lookUpCover(game);
  }

  const noConsoleText = $derived(
    unmatched ? msg('emul.addNoConsole', { ext, list: [...imageExtensions(platforms), ...imageNames(platforms)].join(' ') }) : '',
  );
  const held = $derived(adding ? msg('modals.actionRunning') : '');
</script>

<Modal dismissible={!adding} bind:open title={msg('emul.addTitle')}>
  <div class="form">
    <PathField
      editable
      label={msg('emul.addImageLabel')}
      bind:value={image}
      bind:input={imageInput}
      placeholder={msg('emul.addImagePlaceholder')}
      browseLabel={msg('games.installedBrowseButton')}
      onbrowse={browse}
      hint={hasGameFiles ? msg('emul.addGameFileHint') : undefined}
      error={imageRefused || noConsoleText}
      disabledReason={held || undefined}
    />
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
    <TextField
      label={msg('games.installedTitleFieldLabel')}
      bind:value={title}
      bind:input={titleInput}
      placeholder={msg('games.installedTitlePlaceholder')}
      hint={msg('emul.addTitleHint')}
      error={titleRefused}
      readonly={adding}
      oninput={() => (titleRefused = '')}
    />
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
    <Button disabled={adding} reason={held} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
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

  .form-hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

</style>
