<script lang="ts">
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import { msg } from '../i18n';
  import { snapshotKindLabel, snapshotPlayedAfter, snapshotWhen } from '../savebackup/messages';
  import type { Snapshot } from '../services/savebackup';
  import { bytesSize } from '../utils/format';

  /**
   * One game's local backups in the saves view: "Back up now", the failure of the last action, the
   * list's own failure, the warnings (the game runs, large saves), and each backup with Restore and
   * Delete, or the empty state. It draws what the view holds and reports presses; the view runs the
   * actions. While `busy` no other backup action starts; `creating` is "Back up now" running.
   */
  let {
    snapshots,
    loading = false,
    error = '',
    failure = '',
    running = false,
    busy = false,
    creating = false,
    working,
    canCreate = false,
    createId,
    list = $bindable(),
    oncreate,
    onrestore,
    ondelete,
  }: {
    snapshots: Snapshot[];
    loading?: boolean;
    /** The list could not be loaded. */
    error?: string;
    /** The last backup action failed. */
    failure?: string;
    /** The game runs: nothing is restored meanwhile. */
    running?: boolean;
    busy?: boolean;
    creating?: boolean;
    /** The backup action that runs: a restore or a deletion shows busy on its backup's button. */
    working?: { kind: string; snapshot?: string };
    /** The saves folder is known: a backup can be made. */
    canCreate?: boolean;
    createId?: string;
    /** The list element, for the view to find a row by its backup's id. */
    list?: HTMLElement;
    oncreate: () => void;
    onrestore: (snapshot: Snapshot) => void;
    ondelete: (snapshot: Snapshot) => void;
  } = $props();
</script>

<div class="list">
  <div class="list-head">
    <span class="field-label">{msg('saves.listTitle')}</span>
    <Button id={createId} variant="primary" size="sm" disabled={(busy && !creating) || !canCreate} busy={creating} onclick={oncreate}>
      {creating ? msg('saves.creating') : msg('saves.createBackup')}
    </Button>
  </div>

  {#if failure}
    <Notice tone="danger" compact>{failure}</Notice>
  {/if}
  {#if error}
    <Notice tone="danger" compact>{error}</Notice>
  {/if}
  {#if running}
    <Notice tone="warning" compact announce={false}>{msg('saves.gameRunning')}</Notice>
  {/if}
  {#if snapshots.some((s) => s.large)}
    <Notice tone="warning" compact announce={false}>{msg('saves.largeWarning')}</Notice>
  {/if}

  {#if snapshots.length > 0}
    <ul class="snapshots" bind:this={list}>
      {#each snapshots as snapshot (snapshot.id)}
        {@const restoring = working?.kind === 'restore' && working.snapshot === snapshot.id}
        {@const deleting = working?.kind === 'delete' && working.snapshot === snapshot.id}
        <li class="snapshot" data-snapshot={snapshot.id}>
          <div class="snapshot-main">
            <span class="snapshot-when">{snapshotWhen(snapshot)}</span>
            {#if snapshot.broken}
              <div class="snapshot-broken">
                <StatusBadge kind="danger" label={msg('saves.broken')} />
                {#if snapshot.problem}
                  <span class="snapshot-problem">{snapshot.problem}</span>
                {/if}
              </div>
            {:else}
              <span class="snapshot-meta">
                {snapshotKindLabel(snapshot.kind)} · {bytesSize(snapshot.sizeBytes)} · {msg('saves.filesCount', { count: snapshot.files })}
              </span>
              {#if snapshotPlayedAfter(snapshot)}
                <span class="snapshot-meta">{snapshotPlayedAfter(snapshot)}</span>
              {/if}
            {/if}
          </div>
          <div class="actions">
            {#if !snapshot.broken}
              <Button size="sm" disabled={!restoring && (busy || running)} busy={restoring} onclick={() => onrestore(snapshot)}>
                {msg('saves.restore')}
              </Button>
            {/if}
            <Button size="sm" variant="danger" disabled={!deleting && busy} busy={deleting} onclick={() => ondelete(snapshot)}>
              {msg('saves.delete')}
            </Button>
          </div>
        </li>
      {/each}
    </ul>
  {:else if loading}
    <Spinner size="sm" label={msg('common.loading')} />
  {:else if !error}
    <div class="no-saves">
      <EmptyState size="inline" align="center" lead title={msg('saves.empty')} description={msg('saves.emptyHint')} />
    </div>
  {/if}
</div>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .list-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .snapshots {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .snapshot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    /* The dialog's own surface, so that on the game page a damaged backup's reason reads over the art. */
    background: var(--surface-2);
  }

  .snapshot-main {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-width: 0;
  }

  .snapshot-when {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .snapshot-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .snapshot-broken {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .snapshot-problem {
    font-size: var(--font-xs);
    color: var(--danger);
    word-break: break-word;
  }

  .no-saves {
    padding: var(--space-5) 0;
  }
</style>
