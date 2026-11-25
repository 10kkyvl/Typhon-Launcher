<script lang="ts">
  import { FolderOpen, HardDrive } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DiskMeter from '../../lib/components/DiskMeter.svelte';
  import { gameTone, type MeterSegment } from '../../lib/components/diskMeter';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import { gamesOnVolume } from '../../lib/library/folders';
  import { installedGames } from '../../lib/stores/library';
  import { navigate } from '../../lib/stores/router';
  import { settings } from '../../lib/stores/settings';
  import { refreshStorage, storageFailed, storageInfo } from '../../lib/stores/storage';
  import { bytesSize } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  // The disk of the library folder on the shared meter: each game on it, other files and the free
  // space, or why it is not shown. `onpick` gets the id of a game picked on the meter.
  let { onchoosefolder, onpick }: { onchoosefolder: () => void; onpick: (id: string) => void } = $props();

  // A game that measured nothing takes nothing on the disk; one not fully measured is the meter's
  // "at least" (or "unknown" with no bytes at all).
  const segments = $derived<MeterSegment[]>(
    gamesOnVolume($settings?.libraryPath ?? '', $installedGames)
      .filter((game) => game.sizeBytes > 0 || game.sizeUnknown)
      .toSorted((a, b) => b.sizeBytes - a.sizeBytes || a.title.localeCompare(b.title))
      .map((game, index) => ({ key: game.id, label: game.title, bytes: game.sizeBytes, tone: gameTone(index), sizeUnknown: game.sizeUnknown })),
  );
  const measured = $derived(($storageInfo?.totalBytes ?? 0) > 0);

  let storageRetrying = $state(false);
  let storageCard = $state<HTMLElement>();

  // The notice's retry: it worked when the disk usage came. On success the focus goes to the storage
  // card's "Manage storage".
  async function retryStorage(): Promise<boolean> {
    await refreshStorage();
    return !$storageFailed;
  }
</script>

{#if !$settings?.libraryPath}
  <div class="storage-block">
    <Card>
      <EmptyState size="inline" lead title={msg('games.installedLibraryNotSetTitle')} description={msg('games.installedLibraryNotSetHint')}>
        {#snippet icon()}
          <HardDrive size="1.8rem" strokeWidth={1.8} />
        {/snippet}
        {#snippet actions()}
          <Button size="sm" variant="primary" onclick={onchoosefolder}>
            <FolderOpen size="1.5rem" strokeWidth={1.8} />
            {msg('games.installedChooseFolderButton')}
          </Button>
        {/snippet}
      </EmptyState>
    </Card>
  </div>
{:else if $storageInfo}
  <div class="storage-block">
    <Card>
      <div class="storage" bind:this={storageCard}>
        <div class="storage-head">
          <span class="disk-icon">
            <HardDrive size="1.8rem" strokeWidth={1.8} />
          </span>
          <div class="disk-text">
            <span class="disk-name">{msg('games.installedStorageTitle')}</span>
            {#if measured}
              <span class="disk-meta">
                {msg('games.installedStorageUsage', {
                  used: bytesSize($storageInfo.usedBytes),
                  total: bytesSize($storageInfo.totalBytes),
                })}
                <span class="disk-pct">{Math.round(($storageInfo.usedBytes / $storageInfo.totalBytes) * 100)}%</span>
              </span>
            {/if}
          </div>
          <Button onclick={() => navigate('settings', { tab: 'general' })}>{msg('games.installedManageStorage')}</Button>
        </div>
        <DiskMeter
          {segments}
          total={$storageInfo.totalBytes}
          free={$storageInfo.freeBytes}
          label={$storageInfo.volume ? msg('installed.meterDisk', { volume: $storageInfo.volume }) : msg('games.installedStorageTitle')}
          foldLabel={(count) => msg('installed.meterMoreGames', { count })}
          {onpick}
        />
      </div>
    </Card>
  </div>
{:else if $storageFailed || storageRetrying}
  <div class="storage-block">
    <Notice tone="danger" bind:retrying={storageRetrying} retry={{ label: msg('common.retry'), run: retryStorage, within: () => storageCard }}>
      {msg('games.installedStorageLoadFailed')}
    </Notice>
  </div>
{:else}
  <div class="storage-block">
    <Card>
      <p class="storage-loading">{msg('installed.storageLoading')}</p>
    </Card>
  </div>
{/if}

<style>
  .storage-block {
    margin-bottom: var(--space-6);
  }

  .storage {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .storage-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .storage-head .disk-text {
    flex: 1;
  }

  .disk-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4.4rem;
    height: 4.4rem;
    flex-shrink: 0;
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .disk-text {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .disk-name {
    font-size: var(--font-lg);
    font-weight: 600;
    color: var(--text);
  }

  .disk-meta {
    font-size: var(--font-sm);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .disk-pct {
    margin-left: var(--space-2);
    color: var(--text-2);
  }

  .storage-loading {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
