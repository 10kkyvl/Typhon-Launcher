<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import { addGame, selectExecutable } from '../../lib/services/library';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';
  import { libraryErrorText } from './installedGame';

  // The dialog that adds a game by its executable. `uid` is the page's id prefix: the executable
  // field's label points at an id made from it.
  let { uid }: { uid: string } = $props();

  let addOpen = $state(false);
  let newExecutable = $state('');
  let newTitle = $state('');
  let adding = $state(false);

  function titleFromPath(path: string) {
    const base = path.split(/[\\/]/).pop() ?? '';
    return base.replace(/\.exe$/i, '').replace(/[_.-]+/g, ' ').trim();
  }

  /** Opens the dialog with both fields empty. */
  export function show() {
    newExecutable = '';
    newTitle = '';
    addOpen = true;
  }

  async function browseExecutable() {
    try {
      const path = await selectExecutable(msg('games.installedSelectExeDialog'));
      if (path) {
        newExecutable = path;
        if (!newTitle.trim()) newTitle = titleFromPath(path);
      }
    } catch {
      toast(msg('games.installedOpenDialogError'), 'danger');
    }
  }

  async function submitAdd() {
    if (!newExecutable.trim()) return;
    adding = true;
    try {
      const game = await addGame(newExecutable.trim(), newTitle.trim());
      toast(msg('games.installedGameAddedToast', { title: game.title }), 'success');
      addOpen = false;
    } catch (err) {
      toast(libraryErrorText(err, msg('games.installedAddGameError')), 'danger');
    } finally {
      adding = false;
    }
  }
</script>

<Modal dismissible={!adding} bind:open={addOpen} title={msg('games.installedAddGameModalTitle')}>
  <div class="form">
    <!-- Browse stands beside the field, outside its label: the field is named by the label alone. -->
    <div class="field">
      <label class="field-label" for="{uid}-executable">{msg('games.executableLabel')}</label>
      <div class="field-row">
        <input id="{uid}-executable" class="input" type="text" placeholder="C:\Games\Game\game.exe" bind:value={newExecutable} />
        <Button onclick={browseExecutable}>{msg('games.installedBrowseButton')}</Button>
      </div>
    </div>
    <label class="field">
      <span class="field-label">{msg('games.installedTitleFieldLabel')}</span>
      <input class="input" type="text" placeholder={msg('games.installedTitlePlaceholder')} bind:value={newTitle} />
    </label>
    <p class="form-hint">{msg('games.installedFolderAutoHint')}</p>
  </div>
  {#snippet footer()}
    <Button disabled={adding} onclick={() => (addOpen = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" busy={adding} disabled={!newExecutable.trim() && !adding} onclick={submitAdd}>{msg('common.add')}</Button>
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

  .field-row {
    display: flex;
    gap: 0.8rem;
  }

  .form-hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
