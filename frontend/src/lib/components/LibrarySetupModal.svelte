<script lang="ts">
  import { untrack } from 'svelte';
  import { FolderPlus, HardDrive } from '@lucide/svelte';
  import { proposeLibraryPath, selectFolder, type Settings } from '../services/settings';
  import { getStorageInfoFor, type StorageInfo } from '../services/system';
  import { createLibrary } from '../stores/settings';
  import { toast } from '../stores/toasts';
  import { bytesLabel } from '../utils/format';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    title = msg('modals.librarySetupDefaultTitle'),
    note = '',
    onready = undefined,
  }: { open?: boolean; title?: string; note?: string; onready?: (next: Settings) => void } = $props();

  let parent = $state('');
  let proposed = $state('');
  let info = $state<StorageInfo | null>(null);
  let busy = $state(false);
  let failure = $state('');

  $effect(() => {
    const isOpen = open;
    untrack(() => {
      if (isOpen) return;
      parent = '';
      proposed = '';
      info = null;
      busy = false;
      failure = '';
    });
  });

  function message(err: unknown, fallback: string): string {
    if (err instanceof Error && err.message) return err.message;
    return fallback;
  }

  async function pickFolder() {
    let picked = '';
    try {
      picked = await selectFolder(msg('modals.librarySetupChooseFolder'));
    } catch {
      toast(msg('modals.librarySetupFolderDialogFailed'), 'danger');
      return;
    }
    if (!picked) return;

    failure = '';
    try {
      proposed = await proposeLibraryPath(picked);
      parent = picked;
    } catch (err) {
      proposed = '';
      parent = '';
      info = null;
      failure = message(err, msg('modals.librarySetupFolderUnsuitable'));
      return;
    }
    try {
      info = await getStorageInfoFor(picked);
    } catch (err) {
      info = null;
      failure = message(err, msg('modals.librarySetupFreeSpaceUnknown'));
    }
  }

  async function confirm() {
    if (!parent || busy) return;
    busy = true;
    failure = '';
    try {
      const next = await createLibrary(parent);
      open = false;
      onready?.(next);
    } catch (err) {
      failure = message(err, msg('modals.librarySetupCreateFailed'));
    }
    busy = false;
  }
</script>

<Modal bind:open {title} width="52rem">
  <div class="setup">
    <p class="intro">
      {msg('modals.librarySetupIntro')}
    </p>

    {#if note}
      <p class="note">{note}</p>
    {/if}

    <div class="pick">
      <span class="field-label">{msg('modals.librarySetupFolderLabel')}</span>
      <div class="pick-controls">
        <input class="input sm" type="text" readonly placeholder={msg('modals.librarySetupFolderPlaceholder')} value={proposed} />
        <Button size="sm" onclick={pickFolder}>{msg('modals.librarySetupBrowse')}</Button>
      </div>
    </div>

    {#if info}
      <div class="disk">
        <HardDrive size="1.7rem" strokeWidth={1.8} />
        <div class="disk-text">
          <span class="disk-name">{msg('modals.librarySetupDiskName', { volume: info.volume || '—' })}{info.filesystem ? ` · ${info.filesystem}` : ''}</span>
          <span class="disk-meta">
            {msg('modals.librarySetupDiskFree', { free: bytesLabel(info.freeBytes), total: bytesLabel(info.totalBytes) })}
          </span>
        </div>
      </div>
    {/if}

    {#if failure}
      <span class="failure">{failure}</span>
    {/if}
  </div>

  {#snippet footer()}
    <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={!proposed || busy} onclick={confirm}>
      <FolderPlus size="1.5rem" strokeWidth={1.8} />
      {msg('modals.librarySetupCreate')}
    </Button>
  {/snippet}
</Modal>

<style>
  .setup {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .intro {
    margin: 0;
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text-2);
  }

  .note {
    margin: 0;
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-2);
  }

  .pick {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .pick-controls {
    display: flex;
    gap: var(--space-2);
  }

  .pick-controls .input {
    flex: 1;
  }

  .disk {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    color: var(--text-2);
  }

  .disk-text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .disk-name {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .disk-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .failure {
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
