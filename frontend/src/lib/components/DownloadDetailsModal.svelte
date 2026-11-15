<script lang="ts" module>
  import type { DownloadStatus, FileState } from '../services/downloads';

  /**
   * How often the open dialog reads the files of a running download. Go's progress tick carries no
   * files, and `Get` copies the whole file list (thousands of entries in a large repack) under the
   * manager's lock, so it is read once a second, the rate the screen's speed history samples at, not
   * every tick. Go moves a file's bytes only while it downloads (`Manager.sample`); a verify sets them
   * once as it ends, with a status change, which reads them once more.
   */
  const FILES_READ_MS = 1000;

  const polled: DownloadStatus = 'downloading';
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { installErrorText } from '../install/installErrors';
  import { FolderOpen, Gamepad2 } from '@lucide/svelte';
  import { gamePageId, getDownload } from '../services/downloads';
  import { openFolder } from '../services/settings';
  import { downloadsById, statusLabels } from '../stores/downloads';
  import { navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { bytesSize, dateTime, progressPercent, speedBytes } from '../utils/format';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import { msg } from '../i18n';

  let { open = $bindable(false), id }: { open?: boolean; id: string | null } = $props();
  const uid = $props.id();

  const download = $derived(id ? $downloadsById.get(id) : undefined);
  const gameId = $derived(download ? gamePageId(download) : '');
  const status = $derived(download?.status);
  const running = $derived(status === polled);
  // Only a download that moves data has a speed to show: the record keeps the last one Go sent.
  const takesData = $derived(download?.status === 'downloading');
  const givesData = $derived(download?.status === 'downloading' || Boolean(download?.seeding));

  // The files Go answered with, and the store's file list they were read against: a status event
  // brings a newer list, and the answer then gives way to it.
  // Raw: `base` is compared by identity, which a deep state proxy would break.
  let read = $state.raw<{ files: FileState[]; base: FileState[] } | null>(null);
  // A slow answer is not asked for again while it is on its way. A status change meanwhile is owed a
  // read when it is back: the answer out was asked before the change, and gives way to its list.
  const reading = new Set<string>();
  const owed = new Set<string>();
  const files = $derived(download && read && read.base === download.files ? read.files : (download?.files ?? []));

  async function readFiles(target: string, settled = false) {
    if (reading.has(target)) {
      if (settled) owed.add(target);
      return;
    }
    const base = get(downloadsById).get(target)?.files;
    if (!base) return;
    reading.add(target);
    try {
      const fresh = await getDownload(target);
      if (open && id === target) read = { files: fresh.files, base };
    } catch {
      // A missed read keeps the numbers already shown and says nothing; the next tick asks again.
    } finally {
      reading.delete(target);
      if (owed.delete(target) && open && id === target) void readFiles(target);
    }
  }

  // The dialog's download: what Go answered for another one is not shown.
  let shownFor: string | null = null;

  // One read on open and one on each status change, so the numbers a download stops at are Go's last.
  $effect(() => {
    const target = open ? id : null;
    const now = target ? status : undefined;
    untrack(() => {
      if (target !== shownFor) {
        shownFor = target;
        read = null;
        owed.clear();
      }
      if (target && now) void readFiles(target, true);
    });
  });

  $effect(() => {
    if (!open || !id || !running) return;
    const target = id;
    const timer = setInterval(() => readFiles(target), FILES_READ_MS);
    return () => clearInterval(timer);
  });

  function openGamePage(target: string) {
    open = false;
    navigate('game', { id: target });
  }

  function addedLabel(iso: string) {
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? '—' : dateTime(date);
  }

  async function openDestination(path: string) {
    try {
      await openFolder(path);
    } catch {
      toast(msg('modals.downloadDetailsFolderUnavailable'), 'danger');
    }
  }
</script>

<Modal bind:open title={download?.name ?? msg('modals.downloadDetailsTitle')} width="62rem">
  {#if download}
    <div class="sections">
      <section class="block">
        <h4>{msg('modals.downloadDetailsGeneral')}</h4>
        <div class="rows">
          <div class="row">
            <span class="key">{msg('modals.downloadDetailsStatus')}</span>
            <span class="value">{statusLabels(download.status)}</span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.downloadDetailsFolder')}</span>
            <span class="value path">
              {download.destination}
              <IconButton label={msg('modals.downloadDetailsOpenFolder')} size="sm" onclick={() => openDestination(download.destination)}>
                <FolderOpen size="1.6rem" strokeWidth={1.8} />
              </IconButton>
              {#if gameId}
                <IconButton label={msg('transfers.gamePage')} size="sm" onclick={() => openGamePage(gameId)}>
                  <Gamepad2 size="1.6rem" strokeWidth={1.8} />
                </IconButton>
              {/if}
            </span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.downloadDetailsSize')}</span>
            <span class="value">{bytesSize(download.total)}</span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.downloadDetailsProgress')}</span>
            <span class="value">{msg('format.percent', { value: progressPercent(download.progress) })}</span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.downloadDetailsAdded')}</span>
            <span class="value">{addedLabel(download.addedAt)}</span>
          </div>
          {#if download.error}
            <div class="row">
              <span class="key">{msg('common.error')}</span>
              <span class="value danger">{installErrorText(download.error)}</span>
            </div>
          {/if}
        </div>
      </section>

      <section class="block">
        <h4>{msg('modals.downloadDetailsNetwork')}</h4>
        <div class="rows">
          <div class="row">
            <span class="key">{msg('modals.downloadDetailsSeeds')}</span>
            <span class="value">{download.seeders}</span>
          </div>
          <div class="row">
            <span class="key">{msg('modals.downloadDetailsPeers')}</span>
            <span class="value">{download.peers}</span>
          </div>
          {#if takesData}
            <div class="row">
              <span class="key">{msg('modals.downloadDetailsDownloadSpeed')}</span>
              <span class="value">{speedBytes(download.downloadSpeed)}</span>
            </div>
          {/if}
          {#if givesData}
            <div class="row">
              <span class="key">{msg('modals.downloadDetailsUploadSpeed')}</span>
              <span class="value">{speedBytes(download.uploadSpeed)}</span>
            </div>
          {/if}
        </div>
      </section>

      <section class="block">
        <h4>{msg('modals.downloadDetailsFiles')}</h4>
        <div class="file-list">
          {#each files as file, i (i)}
            {@const amount = file.selected
              ? msg('modals.downloadDetailsFileBytes', { done: bytesSize(file.bytesDone), total: bytesSize(file.size) })
              : msg('modals.downloadDetailsSkipped')}
            <div class="file" class:skipped={!file.selected}>
              <div class="file-head">
                <span class="file-path" id="{uid}-file-{i}">{file.path}</span>
                <!-- Said by the bar below as its value: once. -->
                <span class="file-size" aria-hidden="true">{amount}</span>
              </div>
              <ProgressBar
                value={file.selected && file.size > 0 ? (file.bytesDone / file.size) * 100 : 0}
                height={4}
                tone={file.selected ? 'accent' : 'muted'}
                labelledby="{uid}-file-{i}"
                valueText={amount}
              />
            </div>
          {/each}
        </div>
      </section>
    </div>
  {/if}
</Modal>

<style>
  .sections {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .block h4 {
    font-size: 1.2rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
    margin-bottom: var(--space-2);
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.8rem 0;
    font-size: var(--font-sm);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .key {
    color: var(--text-3);
    flex-shrink: 0;
  }

  .value {
    min-width: 0;
    color: var(--text);
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  .value.path {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
  }

  .value.danger {
    color: var(--danger);
  }

  .file-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    max-height: 26rem;
    overflow-y: auto;
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .file {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .file.skipped {
    opacity: 0.45;
  }

  .file-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .file-path {
    min-width: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-size {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }
</style>
