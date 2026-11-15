<script lang="ts">
  import { FolderOpen, HardDrive } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import { gamesOnVolume } from '../../lib/library/folders';
  import { installedGames } from '../../lib/stores/library';
  import { navigate } from '../../lib/stores/router';
  import { settings } from '../../lib/stores/settings';
  import { refreshStorage, storageFailed, storageInfo } from '../../lib/stores/storage';
  import { bytesSize } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  // The disk of the library folder: what games and everything else take on it, or why it is not shown.
  let { onchoosefolder }: { onchoosefolder: () => void } = $props();

  const volumeGames = $derived(gamesOnVolume($settings?.libraryPath ?? '', $installedGames));
  const gamesBytes = $derived(volumeGames.reduce((sum, game) => sum + game.sizeBytes, 0));
  const hasUnknownSize = $derived(volumeGames.some((game) => game.sizeUnknown));
  const usedPct = $derived($storageInfo ? ($storageInfo.usedBytes / $storageInfo.totalBytes) * 100 : 0);
  const otherBytes = $derived.by(() => {
    if (!$storageInfo || hasUnknownSize) return null;
    const value = $storageInfo.usedBytes - gamesBytes;
    return value >= 0 ? value : null;
  });
  const segments = $derived.by(() => {
    if (!$storageInfo || $storageInfo.totalBytes <= 0) return [];
    if (otherBytes === null) return [{ id: 'used', pct: Math.min(100, usedPct) }];
    const total = $storageInfo.totalBytes;
    return [
      { id: 'games', pct: (gamesBytes / total) * 100 },
      { id: 'other', pct: (otherBytes / total) * 100 },
    ].filter((segment) => segment.pct > 0);
  });

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
            <span class="disk-meta">
              {msg('games.installedStorageUsage', {
                used: bytesSize($storageInfo.usedBytes),
                total: bytesSize($storageInfo.totalBytes),
              })}
            </span>
          </div>
          <Button onclick={() => navigate('settings', { tab: 'general' })}>{msg('games.installedManageStorage')}</Button>
        </div>
        <div class="storage-bar">
          <div class="segments" aria-hidden="true">
            {#each segments as segment (segment.id)}
              <span class="segment {segment.id}" style:width="{segment.pct}%"></span>
            {/each}
            <span class="segment free"></span>
          </div>
          <span class="storage-pct">{Math.round(usedPct)}%</span>
        </div>
        <ul class="storage-legend">
          {#if otherBytes !== null}
            <li>
              <span class="dot games"></span>
              <span class="legend-label">{msg('games.installedLegendGames')}</span>
              <span class="legend-value">{bytesSize(gamesBytes)}</span>
            </li>
            <li>
              <span class="dot other"></span>
              <span class="legend-label">{msg('games.installedLegendOther')}</span>
              <span class="legend-value">{bytesSize(otherBytes)}</span>
            </li>
          {:else}
            <li>
              <span class="dot used"></span>
              <span class="legend-label">{msg('games.installedLegendUsed')}</span>
              <span class="legend-value">{bytesSize($storageInfo.usedBytes)}</span>
            </li>
          {/if}
          <li>
            <span class="dot free"></span>
            <span class="legend-label">{msg('games.installedLegendFree')}</span>
            <span class="legend-value">{bytesSize($storageInfo.freeBytes)}</span>
          </li>
        </ul>
      </div>
    </Card>
  </div>
{:else if $storageFailed || storageRetrying}
  <div class="storage-block">
    <Notice tone="danger" bind:retrying={storageRetrying} retry={{ label: msg('common.retry'), run: retryStorage, within: () => storageCard }}>
      {msg('games.installedStorageLoadFailed')}
    </Notice>
  </div>
{/if}

<style>
  .storage-block {
    margin-bottom: var(--space-6);
  }

  .storage {
    --seg-games: var(--accent-bright);
    --seg-other: var(--text-3);
    --seg-free: color-mix(in srgb, var(--text-3) 28%, transparent);
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
  }

  .storage-bar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .segments {
    display: flex;
    gap: 0.2rem;
    flex: 1;
    min-width: 0;
    height: 0.8rem;
    border-radius: 99rem;
    overflow: hidden;
  }

  .segment {
    min-width: 0.4rem;
    transform-origin: left center;
    animation: segment-grow var(--dur-slow) var(--ease) backwards;
  }

  .segment.games {
    background: var(--seg-games);
  }

  .segment.other {
    background: var(--seg-other);
    animation-delay: calc(var(--dur-fast) / 2);
  }

  .segment.used {
    background: var(--seg-other);
  }

  .segment.free {
    flex: 1;
    min-width: 0;
    background: var(--seg-free);
    animation-name: segment-fade;
  }

  @keyframes segment-grow {
    from {
      transform: scaleX(0);
    }
  }

  @keyframes segment-fade {
    from {
      opacity: 0;
    }
  }

  .storage-pct {
    font-size: var(--font-sm);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .storage-legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-8);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .storage-legend li {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    font-size: var(--font-sm);
  }

  .dot {
    width: 1rem;
    height: 1rem;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .dot.games {
    background: var(--seg-games);
  }

  .dot.other,
  .dot.used {
    background: var(--seg-other);
  }

  .dot.free {
    background: var(--seg-free);
  }

  .legend-label {
    color: var(--text-2);
  }

  .legend-value {
    padding-left: var(--space-2);
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }
</style>
