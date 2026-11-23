<script lang="ts">
  import { FolderOpen, HardDrive } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
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

  const uid = $props.id();
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

  // Unknown before the app facts arrive, and then not Windows: the import is a Windows feature.
  const extlibReason = $derived(page.appInfo?.platform === 'windows' ? undefined : msg('settings.extlibWindowsOnly'));

  async function openPath(path: string | undefined) {
    if (!path) return;
    try {
      await openFolder(path);
    } catch {
      toast(msg('settings.generalLibraryFolderUnavailableToast'), 'danger');
    }
  }
</script>

{#snippet importDetails()}
  <details class="import-details" open>
    <summary>{msg('settings.extlibDetailsTitle')}</summary>
    <ul>
      {#each page.importDetails as line, index (index)}
        <li>{line}</li>
      {/each}
    </ul>
  </details>
{/snippet}

<Card title={msg('settings.generalLibraryCardTitle')}>
  <SettingsRow label={msg('settings.generalLibraryPathLabel')} labelFor="{uid}-library" stack>
    {#snippet children()}
      <div class="path-field">
        <input
          id="{uid}-library"
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
          reason={current?.libraryPath ? '' : msg('settings.generalLibraryOpenNoPathReason')}
          onclick={() => openPath(current?.libraryPath)}
        >
          <FolderOpen size="1.6rem" strokeWidth={1.8} />
        </IconButton>
      </div>
    {/snippet}
  </SettingsRow>
  {#if current?.libraryPath}
    {#each folderRows as folder (folder.key)}
      <SettingsRow label={folder.label} labelFor="{uid}-{folder.key}" stack>
        {#snippet children()}
          <div class="path-field">
            <input id="{uid}-{folder.key}" class="input sm" type="text" readonly value={current?.[folder.key] ?? ''} />
            <IconButton
              label={msg('settings.generalLibraryOpenNamedFolderLabel', { folder: folder.label })}
              size="sm"
              onclick={() => openPath(current?.[folder.key])}
            >
              <FolderOpen size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        {/snippet}
      </SettingsRow>
    {/each}
  {:else}
    <EmptyState size="inline" title={msg('settings.generalLibraryEmptyHint')} />
  {/if}
  {#if current?.libraryPath}
    <LibraryFoldersRow />
  {/if}
  <SettingsRow label={msg('diskusage.settingsLabel')} hint={msg('diskusage.settingsSub')}>
    {#snippet children({ hintId })}
      <Button size="sm" aria-describedby={hintId} onclick={() => navigate('diskusage')}>
        <HardDrive size="1.5rem" strokeWidth={1.8} />
        {msg('diskusage.settingsOpen')}
      </Button>
    {/snippet}
  </SettingsRow>
  <SettingsRow
    label={msg('settings.extlibLabel')}
    hint={msg('settings.extlibSub')}
    disabledReason={extlibReason}
    below={page.importDetails.length > 0 ? importDetails : undefined}
  >
    {#snippet children({ hintId, disabled })}
      <Button size="sm" aria-describedby={hintId} {disabled} busy={page.importing} onclick={page.runImportExternal}>
        {page.importing ? msg('settings.extlibRunning') : msg('settings.extlibLabel')}
      </Button>
    {/snippet}
  </SettingsRow>
  <LibraryLocationRow />
</Card>

<style>
  /* A path field fills the row's measure; its buttons keep their own width beside it. */
  .path-field {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
  }

  .path-field input {
    flex: 1;
    min-width: 0;
  }

  .import-details summary {
    cursor: pointer;
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .import-details ul {
    margin: var(--space-2) 0 0;
    padding-left: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
