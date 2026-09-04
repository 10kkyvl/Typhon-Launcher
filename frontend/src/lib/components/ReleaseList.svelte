<script lang="ts">
  import { Download } from '@lucide/svelte';
  import { languageLabel } from '../game/view';
  import type { ReleaseGroup } from '../services/sources';
  import { bytesSize, relativeDate } from '../utils/format';
  import { msg } from '../i18n';
  import Button from './Button.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let {
    groups,
    loading = false,
    currentReleaseId = '',
    updateReleaseId = '',
    ondownload,
  }: {
    groups: ReleaseGroup[];
    loading?: boolean;
    currentReleaseId?: string;
    updateReleaseId?: string;
    ondownload: (group: ReleaseGroup) => void;
  } = $props();

  const showLanguages = $derived(groups.some((group) => languageLabel(group.release.languages) !== ''));
</script>

<div class="release-list">
  {#if loading && groups.length === 0}
    <p class="hint">Загрузка релизов…</p>
  {:else}
    {#each groups as group (group.release.id)}
      {@const release = group.release}
      {@const removed = release.availability === 'removed'}
      {@const current = Boolean(currentReleaseId) && release.id === currentReleaseId}
      <div class="release-row" class:current>
        <div class="release-main">
          <span class="release-version">{release.version || '—'}</span>
          {#if release.edition}
            <span class="release-edition">{release.edition}</span>
          {/if}
        </div>
        <span class="release-size"
          >{bytesSize(release.size)}{#if release.dlcCount} +{release.dlcCount} DLC{/if}</span
        >
        {#if showLanguages}
          <span class="release-lang">{languageLabel(release.languages)}</span>
        {/if}
        <span class="release-source">
          {group.sourceName}
          {#if group.duplicates && group.duplicates.length > 0}
            <span
              class="release-dup"
              title={group.duplicates.map((d) => d.sourceName).join(', ')}
            >
              +{group.duplicates.length}
              {msg('release.duplicateSources', { count: group.duplicates.length })}
            </span>
          {/if}
        </span>
        <span class="release-date">{relativeDate(release.uploadedAt)}</span>
        <div class="release-badges">
          {#if release.repacker}
            <StatusBadge kind="neutral" label={release.repacker.toUpperCase()} plain />
          {/if}
          {#if current}
            <StatusBadge kind="success" label="Установлено" plain />
          {:else if updateReleaseId && release.id === updateReleaseId}
            <StatusBadge kind="accent" label="Обновление" plain />
          {:else if release.new}
            <StatusBadge kind="accent" label="Новое" plain />
          {/if}
          {#if removed}
            <StatusBadge kind="danger" label="Недоступно" plain />
          {/if}
        </div>
        <Button size="sm" disabled={removed} onclick={() => ondownload(group)}>
          <Download size="1.4rem" strokeWidth={1.8} />
          Скачать
        </Button>
      </div>
    {/each}
  {/if}
</div>

<style>
  .release-list {
    display: flex;
    flex-direction: column;
  }

  .hint {
    font-size: var(--font-sm);
    color: var(--text-3);
    padding: 0.8rem 0;
  }

  .release-row {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 1.1rem 0;
  }

  .release-row + .release-row {
    border-top: 1px solid var(--border);
  }

  .release-main {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1.3;
    gap: 2px;
  }

  .release-version {
    font-size: var(--font-sm);
    font-weight: 500;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .release-edition {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .release-size {
    flex: 0 0 7rem;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .release-row.current .release-version {
    color: var(--text);
  }

  .release-row.current::before {
    content: '';
    position: absolute;
    left: -1.2rem;
    top: 1.2rem;
    bottom: 1.2rem;
    width: 2px;
    border-radius: 2px;
    background: var(--success);
  }

  .release-lang {
    flex: 0 0 9rem;
    min-width: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .release-source {
    flex: 1;
    min-width: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .release-dup {
    margin-left: 0.6rem;
    color: var(--accent-text);
  }

  .release-date {
    flex: 0 0 8.5rem;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  .release-badges {
    display: flex;
    gap: 0.6rem;
    flex-shrink: 0;
  }
</style>
