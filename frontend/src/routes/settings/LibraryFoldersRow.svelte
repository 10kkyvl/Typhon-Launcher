<script lang="ts">
  import { Plus } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
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
  // Where focus goes once the removed folder's row, and its Remove, are gone.
  let nextAfterRemove: string | null = null;
  let list = $state<HTMLUListElement>();
  const uid = $props.id();
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

  function askRemove(path: string) {
    const at = folders.findIndex((folder) => folder.path === path);
    nextAfterRemove = folders.slice(at + 1).find((folder) => !folder.primary)?.path ?? null;
    pendingRemove = path;
  }

  function focusAfterRemove(): HTMLElement | null {
    const next = [...(list?.querySelectorAll('li') ?? [])].find((item) => item.dataset.path === nextAfterRemove);
    return next?.querySelector('button') ?? document.getElementById(`${uid}-add`);
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
  <SettingsRow label={msg('settings.libraryFoldersLabel')} hint={msg('settings.libraryFoldersSub')}>
    {#snippet children({ hintId })}
      <Button id="{uid}-add" size="sm" aria-describedby={hintId} busy={adding} onclick={add}>
        <Plus size="1.5rem" strokeWidth={1.8} />
        {msg('settings.libraryFolderAdd')}
      </Button>
    {/snippet}
    {#snippet below({ labelId })}
      {#if loadFailed && folders.length === 0}
        <Notice tone="danger" compact>{msg('settings.libraryFoldersLoadFailed')}</Notice>
      {/if}
      {#if folders.length > 0}
        <ul class="folders" aria-labelledby={labelId} bind:this={list}>
          {#each folders as folder (folder.path)}
            <li class="folder" class:unavailable={!folder.available} data-path={folder.path}>
              <div class="folder-text">
                <span class="path">{folder.path}</span>
                <span class="facts">
                  {#if folder.primary}<StatusBadge kind="accent" dot={false} label={msg('settings.libraryFolderPrimary')} />{/if}
                  {#if folder.available}
                    <span>{msg('settings.libraryFolderSpace', { free: bytesSize(folder.freeBytes), total: bytesSize(folder.totalBytes) })}</span>
                  {:else}
                    <StatusBadge kind="danger" label={msg('settings.libraryFolderUnavailable')} />
                  {/if}
                  <span>{msg('settings.libraryFolderGames', { count: gamesInFolder(folder.path, $installedGames) })}</span>
                </span>
              </div>
              {#if !folder.primary}
                <Button size="sm" disabled={removing === folder.path} onclick={() => askRemove(folder.path)}>
                  {msg('settings.libraryFolderRemove')}
                </Button>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    {/snippet}
  </SettingsRow>
{/if}

{#if pendingRemove}
  {@const target = pendingRemove}
  <ConfirmModal
    prompt={removeLibraryFolderPrompt(target)}
    onconfirm={() => remove(target)}
    onclose={() => (pendingRemove = null)}
    returnFocus={focusAfterRemove}
  />
{/if}

<style>
  .folders {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .folder {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-3);
  }

  .folder + .folder {
    border-top: 1px solid var(--border);
  }

  .folder-text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .path {
    font-size: var(--font-sm);
    overflow-wrap: anywhere;
  }

  .folder.unavailable .path {
    color: var(--text-3);
  }

  /* The folder's facts, apart by space rather than by a joining glyph. */
  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
