<script lang="ts">
  import { FolderOpen, HardDrive, RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import LibraryFoldersRow from './LibraryFoldersRow.svelte';
  import LibraryLocationRow from './LibraryLocationRow.svelte';
  import { msg } from '../../lib/i18n';
  import { openFolder } from '../../lib/services/settings';
  import { navigate } from '../../lib/stores/router';
  import { settings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import type { SettingsPage } from './settingsPage.svelte';

  // The library card of the general tab: the library folder and its parts, the disk usage link and
  // the import of games from other launchers. The import and its details are the page's: they
  // outlive a switch of tabs, and the library setup dialog is the page's.
  let { page }: { page: SettingsPage } = $props();

  const current = $derived($settings);

  type PathKey = 'gamesPath' | 'downloadsPath' | 'screenshotsPath';

  function folderRowOptions(): { key: PathKey; label: string }[] {
    return [
      { key: 'gamesPath', label: msg('settings.generalLibraryFolderGames') },
      { key: 'downloadsPath', label: msg('settings.generalLibraryFolderDownloads') },
      { key: 'screenshotsPath', label: msg('settings.generalLibraryFolderScreenshots') },
    ];
  }

  const folderRows = $derived(folderRowOptions());

  async function openPath(path: string | undefined) {
    if (!path) return;
    try {
      await openFolder(path);
    } catch {
      toast(msg('settings.generalLibraryFolderUnavailableToast'), 'danger');
    }
  }
</script>

<Card title={msg('settings.generalLibraryCardTitle')}>
  <div class="rows">
    <div class="row folder-row">
      <span class="row-label folder-label">{msg('settings.generalLibraryPathLabel')}</span>
      <div class="folder-controls">
        <input
          class="input sm"
          type="text"
          readonly
          placeholder={msg('settings.generalLibraryPathPlaceholder')}
          value={current?.libraryPath ?? ''}
        />
        <Button size="sm" onclick={page.openLibrarySetup}>
          {current?.libraryPath ? msg('common.edit') : msg('common.select')}
        </Button>
        <IconButton
          label={msg('settings.generalLibraryOpenFolderLabel')}
          size="sm"
          disabled={!current?.libraryPath}
          onclick={() => openPath(current?.libraryPath)}
        >
          <FolderOpen size="1.6rem" strokeWidth={1.8} />
        </IconButton>
      </div>
    </div>
    {#if current?.libraryPath}
      {#each folderRows as folder (folder.key)}
        <div class="row folder-row">
          <span class="row-label folder-label">{folder.label}</span>
          <div class="folder-controls">
            <input class="input sm" type="text" readonly value={current?.[folder.key] ?? ''} />
            <IconButton label={msg('settings.generalLibraryOpenFolderLabel')} size="sm" onclick={() => openPath(current?.[folder.key])}>
              <FolderOpen size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        </div>
      {/each}
    {:else}
      <EmptyState size="inline" title={msg('settings.generalLibraryEmptyHint')} />
    {/if}
    {#if current?.libraryPath}
      <LibraryFoldersRow />
    {/if}
    <div class="row">
      <div class="row-text">
        <span class="row-label">{msg('diskusage.settingsLabel')}</span>
        <span class="row-sub">{msg('diskusage.settingsSub')}</span>
      </div>
      <Button size="sm" onclick={() => navigate('diskusage')}>
        <HardDrive size="1.5rem" strokeWidth={1.8} />
        {msg('diskusage.settingsOpen')}
      </Button>
    </div>
    <div class="row">
      <div class="row-text">
        <span class="row-label">{msg('settings.extlibLabel')}</span>
        <span class="row-sub">{msg('settings.extlibSub')}</span>
      </div>
      {#if page.appInfo?.platform === 'windows'}
        <Button size="sm" disabled={page.importing} onclick={page.runImportExternal}>
          <RefreshCw size="1.5rem" strokeWidth={1.8} />
          {page.importing ? msg('settings.extlibRunning') : msg('settings.extlibLabel')}
        </Button>
      {:else}
        <span class="row-sub">{msg('settings.extlibWindowsOnly')}</span>
      {/if}
    </div>
    {#if page.importDetails.length > 0}
      <details class="import-details" open>
        <summary class="row-label">{msg('settings.extlibDetailsTitle')}</summary>
        <ul>
          {#each page.importDetails as line, index (index)}
            <li class="row-sub">{line}</li>
          {/each}
        </ul>
      </details>
    {/if}
    <LibraryLocationRow />
  </div>
</Card>

<style>
  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
  }

  .row + .row {
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

  .import-details {
    padding: var(--space-3) 0;
  }

  .import-details summary {
    cursor: pointer;
  }

  .import-details ul {
    margin: var(--space-2) 0 0;
    padding-left: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .folder-row {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-2);
  }

  .folder-label {
    font-size: var(--font-sm);
  }

  .folder-controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
</style>
