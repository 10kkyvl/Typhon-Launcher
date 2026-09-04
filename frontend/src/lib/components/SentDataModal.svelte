<script lang="ts">
  import { Copy } from '@lucide/svelte';
  import { dateTime } from '../utils/format';
  import { listSentData, type SentDataEntry } from '../services/telemetryLog';
  import { toast } from '../stores/toasts';
  import EmptyState from './EmptyState.svelte';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import { msg } from '../i18n';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  let entries = $state<SentDataEntry[]>([]);
  let loading = $state(false);
  let failed = $state(false);

  $effect(() => {
    if (open) load();
  });

  async function load() {
    loading = true;
    failed = false;
    try {
      entries = await listSentData();
    } catch {
      entries = [];
      failed = true;
    } finally {
      loading = false;
    }
  }

  function kindLabel(kind: string): string {
    if (kind === 'diagnostics') return msg('modals.sentDataKindDiagnostics');
    if (kind === 'usagestats') return msg('modals.sentDataKindUsageStats');
    return kind || '—';
  }

  function timeLabel(iso: string): string {
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? '—' : dateTime(date);
  }

  async function copyPayload(entry: SentDataEntry) {
    try {
      await navigator.clipboard.writeText(entry.payload);
      toast(msg('modals.sentDataCopied'), 'info');
    } catch {
      toast(msg('modals.sentDataCopyFailed'), 'danger');
    }
  }
</script>

<Modal bind:open title={msg('modals.sentDataTitle')} width="76rem">
  {#if loading}
    <p class="status">{msg('modals.sentDataLoading')}</p>
  {:else if failed}
    <p class="status error">{msg('modals.sentDataLoadFailed')}</p>
  {:else if entries.length === 0}
    <EmptyState
      title={msg('modals.sentDataEmptyTitle')}
      description={msg('modals.sentDataEmptyDescription')}
    />
  {:else}
    <div class="list">
      {#each entries as entry, i (i)}
        <div class="entry">
          <div class="entry-head">
            <div class="entry-meta">
              <StatusBadge kind={entry.kind === 'diagnostics' ? 'accent' : 'neutral'} label={kindLabel(entry.kind)} />
              <span class="time">{timeLabel(entry.sentAt)}</span>
              <span class="endpoint" title={entry.endpoint}>{entry.endpoint}</span>
            </div>
            <IconButton label={msg('modals.sentDataCopyLabel')} size="sm" onclick={() => copyPayload(entry)}>
              <Copy size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          </div>
          {#if !entry.formatted}
            <p class="warn">{msg('modals.sentDataTruncatedWarning')}</p>
          {/if}
          <pre class="payload">{entry.payload}</pre>
        </div>
      {/each}
    </div>
  {/if}
</Modal>

<style>
  .status {
    padding: var(--space-6) 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .status.error {
    color: var(--danger);
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .entry {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .entry-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .entry-meta {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
  }

  .time {
    font-size: var(--font-xs);
    color: var(--text-3);
    flex-shrink: 0;
    font-variant-numeric: tabular-nums;
  }

  .endpoint {
    font-size: var(--font-xs);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .warn {
    font-size: var(--font-xs);
    color: var(--warning);
    background: var(--warning-subtle);
    padding: 0.6rem var(--space-3);
    border-radius: var(--radius-sm);
  }

  .payload {
    max-width: 100%;
    max-height: 32rem;
    overflow: auto;
    margin: 0;
    padding: var(--space-3) var(--space-4);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
    font-size: var(--font-xs);
    line-height: 1.5;
    white-space: pre;
    color: var(--text-2);
  }
</style>
