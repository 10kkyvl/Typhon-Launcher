<script lang="ts">
  import { onDestroy } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import PathField from '../../lib/components/PathField.svelte';
  import TextField from '../../lib/components/TextField.svelte';
  import { addGame, selectExecutable, type LibraryGame } from '../../lib/services/library';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';
  import { libraryErrorText } from './installedGame';

  // The dialog that adds a game by its executable. `uid` is the page's id prefix: the executable
  // field's id is made from it. `onadded` hears of the game before the dialog closes, so `returnFocus`
  // can find it when the dialog's opener has gone. A refusal is said at the executable field. The
  // dialog is opened again with `show`: an answer for an earlier opening is not this one's, and is said
  // once in a toast naming its game.
  let {
    uid,
    returnFocus,
    onadded,
  }: {
    uid: string;
    returnFocus?: () => HTMLElement | null | undefined;
    onadded?: (game: LibraryGame) => void;
  } = $props();

  let addOpen = $state(false);
  let newExecutable = $state('');
  let newTitle = $state('');
  let adding = $state(false);
  let failure = $state('');
  let opening = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  function titleFromPath(path: string) {
    const base = path.split(/[\\/]/).pop() ?? '';
    return base.replace(/\.exe$/i, '').replace(/[_.-]+/g, ' ').trim();
  }

  /** Opens the dialog with both fields empty. */
  export function show() {
    opening += 1;
    newExecutable = '';
    newTitle = '';
    adding = false;
    failure = '';
    addOpen = true;
  }

  async function browseExecutable() {
    const at = opening;
    try {
      const path = await selectExecutable(msg('games.installedSelectExeDialog'));
      if (path && alive && at === opening) {
        newExecutable = path;
        failure = '';
        if (!newTitle.trim()) newTitle = titleFromPath(path);
      }
    } catch (err) {
      console.warn('add game: executable picker', err);
      if (alive && at === opening) failure = msg('games.installedOpenDialogError');
    }
  }

  async function submitAdd() {
    if (!newExecutable.trim() || adding) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const executable = newExecutable.trim();
    const title = newTitle.trim();
    adding = true;
    failure = '';
    try {
      const game = await addGame(executable, title);
      toast(msg('games.installedGameAddedToast', { title: game.title }), 'success');
      if (!heard()) return;
      onadded?.(game);
      addOpen = false;
    } catch (err) {
      const reason = libraryErrorText(err, msg('games.installedAddGameError'));
      if (!heard()) {
        toast(msg('games.installedAddGameFailedFor', { title: title || titleFromPath(executable), reason }), 'danger');
        return;
      }
      failure = reason;
    } finally {
      if (heard()) adding = false;
    }
  }

  const heldReason = $derived(adding ? msg('modals.actionRunning') : '');
  const addReason = $derived(adding || newExecutable.trim() ? '' : msg('games.installedSelectExeDialog'));
</script>

<Modal dismissible={!adding} bind:open={addOpen} title={msg('games.installedAddGameModalTitle')} {returnFocus}>
  <div class="form">
    <PathField
      id="{uid}-executable"
      label={msg('games.executableLabel')}
      editable
      bind:value={newExecutable}
      placeholder="C:\Games\Game\game.exe"
      browseLabel={msg('games.installedBrowseButton')}
      onbrowse={browseExecutable}
      error={failure}
      disabledReason={heldReason || undefined}
    />
    <TextField label={msg('games.installedTitleFieldLabel')} placeholder={msg('games.installedTitlePlaceholder')} bind:value={newTitle} readonly={adding} />
    <p class="form-hint">{msg('games.installedFolderAutoHint')}</p>
  </div>
  {#snippet footer()}
    <Button disabled={adding} reason={heldReason} onclick={() => (addOpen = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" busy={adding} disabled={!!addReason} reason={addReason} onclick={submitAdd}>{msg('common.add')}</Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .form-hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
