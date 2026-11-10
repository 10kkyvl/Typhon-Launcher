<script lang="ts">
  import { Plus } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import { removeLibraryFolderPrompt } from '../../lib/confirm/prompts';
  import { errorCode, hasMessage, msg } from '../../lib/i18n';
  import { gamesInFolder } from '../../lib/library/folders';
  import { inWails } from '../../lib/services/backend';
  import {
    addLibraryFolder,
    libraryFolders,
    removeLibraryFolder,
    selectFolder,
    type LibraryFolder,
  } from '../../lib/services/settings';
  import { installedGames } from '../../lib/stores/library';
  import { settings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { bytesSize } from '../../lib/utils/format';

  let folders = $state<LibraryFolder[]>([]);
  let loadFailed = $state(false);
  let adding = $state(false);
  let removing = $state<string | null>(null);
  let pendingRemove = $state<string | null>(null);
  let seq = 0;

  function errorText(err: unknown, fallback: string): string {
    const code = errorCode(err);
    return hasMessage(code) ? msg(code) : fallback;
  }

  async function refresh() {
    const mine = ++seq;
    try {
      const next = await libraryFolders();
      if (mine !== seq) return;
      folders = next;
      loadFailed = false;
    } catch (err) {
      if (mine !== seq) return;
      loadFailed = true;
      toast(errorText(err, msg('settings.libraryFoldersLoadFailed')), 'danger');
    }
  }

  $effect(() => {
    void $settings?.gamesPath;
    void $settings?.extraLibraryPaths?.join('\n');
    untrack(() => {
      void refresh();
    });
  });

  async function add() {
    if (adding) return;
    let path = '';
    try {
      path = await selectFolder(msg('settings.libraryFolderChoose'));
    } catch {
      toast(msg('settings.libraryFolderDialogFailed'), 'danger');
      return;
    }
    if (!path) return;
    adding = true;
    try {
      settings.set(await addLibraryFolder(path));
      await refresh();
    } catch (err) {
      toast(errorText(err, msg('settings.libraryFolderAddFailed')), 'danger');
    } finally {
      adding = false;
    }
  }

  async function remove(path: string) {
    removing = path;
    try {
      settings.set(await removeLibraryFolder(path));
      await refresh();
    } catch (err) {
      toast(errorText(err, msg('settings.libraryFolderRemoveFailed')), 'danger');
    } finally {
      removing = null;
    }
  }
</script>

{#if inWails}
  <div class="row">
    <div class="row-text">
      <span class="row-label">{msg('settings.libraryFoldersLabel')}</span>
      <span class="row-sub">{msg('settings.libraryFoldersSub')}</span>
    </div>
    <Button size="sm" disabled={adding} onclick={add}>
      <Plus size="1.5rem" strokeWidth={1.8} />
      {msg('settings.libraryFolderAdd')}
    </Button>
  </div>

  {#if loadFailed && folders.length === 0}
    <p class="failure" role="alert">{msg('settings.libraryFoldersLoadFailed')}</p>
  {/if}

  {#each folders as folder (folder.path)}
    <div class="row folder" class:unavailable={!folder.available}>
      <div class="row-text">
        <span class="path" title={folder.path}>{folder.path}</span>
        <span class="row-sub">
          {#if folder.primary}<span class="tag">{msg('settings.libraryFolderPrimary')}</span>{/if}
          {#if folder.available}
            {msg('settings.libraryFolderSpace', { free: bytesSize(folder.freeBytes), total: bytesSize(folder.totalBytes) })}
          {:else}
            <span class="warn">{msg('settings.libraryFolderUnavailable')}</span>
          {/if}
          · {msg('settings.libraryFolderGames', { count: gamesInFolder(folder.path, $installedGames) })}
        </span>
      </div>
      {#if !folder.primary}
        <Button size="sm" disabled={removing === folder.path} onclick={() => (pendingRemove = folder.path)}>
          {msg('settings.libraryFolderRemove')}
        </Button>
      {/if}
    </div>
  {/each}
{/if}

{#if pendingRemove}
  {@const target = pendingRemove}
  <ConfirmModal
    prompt={removeLibraryFolderPrompt(target)}
    onconfirm={() => remove(target)}
    onclose={() => (pendingRemove = null)}
  />
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
    border-top: 1px solid var(--border);
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .row-label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .row-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .path {
    font-size: var(--font-sm);
    overflow-wrap: anywhere;
    user-select: text;
  }

  .folder.unavailable .path {
    color: var(--text-3);
  }

  .tag {
    margin-right: var(--space-2);
    color: var(--accent-text);
    font-weight: 500;
  }

  .warn {
    color: var(--danger);
  }

  .failure {
    margin: var(--space-2) 0 0;
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
