<script lang="ts">
  import { FolderOpen, Gamepad2, X } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { gamePageId } from '../../lib/services/downloads';
  import { openFolder } from '../../lib/services/settings';
  import { completed, remove } from '../../lib/stores/downloads';
  import { installIndeterminate } from '../../lib/install/progress';
  import { installActive, installStatusLabels, installationsByDownload } from '../../lib/stores/install';
  import { gameArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { sources } from '../../lib/stores/sources';
  import { toast } from '../../lib/stores/toasts';
  import { bytesSize, clockTime, relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import DownloadsSection from './DownloadsSection.svelte';
  import { coverOf, sourceTag, typeTag } from './downloadRow';

  let { onopen, oninstall }: { onopen: (id: string) => void; oninstall: (id: string) => void } = $props();

  function completedWhen(iso: string | null) {
    if (!iso) return '—';
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '—';
    const time = clockTime(date);
    const rel = relativeDate(iso);
    return msg('transfers.downloadsCompletedAt', { rel: `${rel.charAt(0).toLowerCase()}${rel.slice(1)}`, time });
  }

  async function openDestination(path: string) {
    try {
      await openFolder(path);
    } catch {
      toast(msg('transfers.downloadsFolderUnavailable'), 'danger');
    }
  }
</script>

{#if $completed.length > 0}
<DownloadsSection title={msg('transfers.downloadsCompletedHeading')} count={$completed.length}>
  <div class="rows">
    {#each $completed as item (item.id)}
      {@const install = $installationsByDownload.get(item.id)}
      <div class="row">
        <div class="thumb" aria-hidden="true">
          <Artwork src={coverOf(item, $gameArt)} alt="" label={item.name} ratio="3 / 4" radius="var(--radius-sm)" />
        </div>
        <div class="info inline">
          <span class="title wraps">
            <TextLink tone="muted" onclick={() => onopen(item.id)}>{item.name}</TextLink>
          </span>
          {#if typeTag(item) || sourceTag(item, $sources)}
            <div class="tags">
              {#if typeTag(item)}<StatusBadge kind="neutral" label={typeTag(item)} dot={false} />{/if}
              {#if item.origin.platform}<ConsoleTag platform={item.origin.platform} />{/if}
              {#if sourceTag(item, $sources)}<StatusBadge kind="neutral" label={sourceTag(item, $sources)} dot={false} />{/if}
            </div>
          {/if}
        </div>
        <div class="status" title={msg('transfers.downloadsDoneWhen', { when: completedWhen(item.completedAt) })}>
          <span class="status-main">{bytesSize(item.total)}</span>
          <span class="status-sub">{completedWhen(item.completedAt)}</span>
        </div>
        <div class="install-cell">
          {#if !install}
            <Button size="sm" variant="primary" onclick={() => oninstall(item.id)}>{msg('transfers.downloadsInstallAction')}</Button>
          {:else if installActive(install.status)}
            <div class="install-progress">
              <span class="install-status">{installStatusLabels(install.status)}</span>
              <ProgressBar value={install.progress * 100} indeterminate={installIndeterminate(install)} height={4} label={item.name} />
            </div>
          {:else if install.status === 'waiting_for_user'}
            <Button size="sm" variant="primary" onclick={() => oninstall(item.id)}>{msg('transfers.downloadsContinueInstallAction')}</Button>
          {:else if install.status === 'completed'}
            {#if install.warning}
              <StatusBadge kind="warning" label={msg('transfers.downloadsInstalledStatus')} hint={install.warning} plain />
            {:else}
              <StatusBadge kind="success" label={msg('transfers.downloadsInstalledStatus')} plain />
            {/if}
          {:else if install.status === 'failed'}
            <Button size="sm" variant="danger" onclick={() => oninstall(item.id)}>{msg('transfers.downloadsInstallErrorAction')}</Button>
          {:else}
            <Button size="sm" onclick={() => oninstall(item.id)}>
              {install.status === 'cancelled' ? msg('transfers.downloadsCancelledStatus') : msg('transfers.downloadsInterruptedStatus')}
            </Button>
          {/if}
        </div>
        <div class="row-actions">
          {#if gamePageId(item)}
            <IconButton label={msg('transfers.gamePage')} size="sm" onclick={() => navigate('game', { id: gamePageId(item) })}>
              <Gamepad2 size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          {/if}
          <IconButton label={msg('transfers.downloadsShowInFolderLabel')} size="sm" onclick={() => openDestination(item.destination)}>
            <FolderOpen size="1.5rem" strokeWidth={1.8} />
          </IconButton>
          <IconButton label={msg('transfers.downloadsRemoveFromListLabel')} size="sm" onclick={() => remove(item.id)}>
            <X size="1.5rem" strokeWidth={1.8} />
          </IconButton>
        </div>
      </div>
    {/each}
  </div>
</DownloadsSection>
{/if}

<style>
  .rows {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    min-height: 6.4rem;
    padding: var(--space-2) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    transition: border-color var(--dur) var(--ease);
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .row:hover {
    border-color: var(--border-strong);
  }

  .thumb {
    width: 3.6rem;
    flex-shrink: 0;
  }

  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
  }

  .info.inline {
    flex-direction: row;
    align-items: center;
    gap: var(--space-3);
  }

  .tags {
    flex-shrink: 0;
  }

  .title {
    min-width: 0;
    max-width: 100%;
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* A name is shown whole, wrapping in its row: an ellipsis would hide all of a text link (a button,
     an atomic box), and a hint on the rest would reach a mouse only. */
  .title.wraps {
    white-space: normal;
    overflow: visible;
    overflow-wrap: anywhere;
  }

  .tags {
    display: flex;
    gap: 0.6rem;
  }

  .status {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    flex-shrink: 0;
    width: 17rem;
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
  }

  .status-main {
    color: var(--text-2);
  }

  .status-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .row-actions {
    display: flex;
    gap: 0.4rem;
    flex-shrink: 0;
  }

  .install-cell {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    width: 17rem;
    flex-shrink: 0;
  }

  .install-progress {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    width: 100%;
  }

  .install-status {
    font-size: var(--font-xs);
    color: var(--text-3);
    text-align: right;
  }

  @media (max-width: 1300px) {
    .status {
      display: none;
    }
  }
</style>
