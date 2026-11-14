<script lang="ts" module>
  /** Whether the game can have a cloud copy now: cloud saves off, signed out, unknown to the catalog, or on. */
  export type CloudAvailability = 'off' | 'signed-out' | 'no-catalog' | 'on';
</script>

<script lang="ts">
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import { cloudCodeText, cloudStateKind, cloudStateLabel } from '../cloudsaves/messages';
  import { msg } from '../i18n';
  import type { CloudGameStatus } from '../services/cloudsaves';
  import { dateTime } from '../utils/format';

  /**
   * The cloud copy of one game in the saves view: its state, when it last synced, why it is held, and
   * its actions. It draws what the view holds and reports presses; the view runs the actions.
   * `busy` while Sync now, Pull or Delete remote runs; `running` while the game is (no Pull then).
   */
  let {
    availability,
    game,
    failure = '',
    busy = false,
    running = false,
    syncId,
    onsettings,
    onsync,
    onresolve,
    onpull,
    ondelete,
  }: {
    availability: CloudAvailability;
    game?: Pick<CloudGameStatus, 'state' | 'code' | 'syncedAt'>;
    failure?: string;
    busy?: boolean;
    running?: boolean;
    /** The id of Sync now, where focus goes when Resolve goes. */
    syncId?: string;
    onsettings: () => void;
    onsync: () => void;
    onresolve: () => void;
    onpull: () => void;
    ondelete: () => void;
  } = $props();

  const reason = $derived(game?.code ? cloudCodeText(game.code, '') : '');
  const syncedAt = $derived.by(() => {
    const value = game?.syncedAt ? new Date(game.syncedAt) : null;
    return value && !Number.isNaN(value.getTime()) && value.getFullYear() > 1 ? dateTime(value) : '';
  });
</script>

{#if availability === 'off'}
  <p class="hint">{msg('cloudsaves.disabledHint')}</p>
  <div class="actions">
    <Button size="sm" onclick={onsettings}>{msg('cloudsaves.openSettings')}</Button>
  </div>
{:else if availability === 'signed-out'}
  <p class="hint">{msg('cloudsaves.needsAccount')}</p>
{:else if availability === 'no-catalog'}
  <p class="hint">{cloudCodeText('cloud.no_canonical_id', msg('cloudsaves.fallback'))}</p>
{:else}
  <div class="cloud-state">
    {#if game}
      <StatusBadge kind={cloudStateKind(game.state)} label={cloudStateLabel(game.state)} />
    {:else}
      <StatusBadge kind="neutral" label={msg('cloudsaves.stateNone')} />
    {/if}
    {#if syncedAt}
      <span class="snapshot-meta">{msg('cloudsaves.syncedAt', { date: syncedAt })}</span>
    {/if}
  </div>
  {#if reason}
    <Notice tone={game?.state === 'failed' ? 'danger' : 'info'} compact announce={false}>{reason}</Notice>
  {/if}
  {#if failure}
    <Notice tone="danger" compact>{failure}</Notice>
  {/if}
  <div class="actions">
    <Button id={syncId} size="sm" {busy} onclick={onsync}>
      {busy ? msg('cloudsaves.syncing') : game?.code === 'cloud.excluded' ? msg('cloudsaves.enableAgain') : msg('cloudsaves.syncNow')}
    </Button>
    {#if game?.state === 'conflict'}
      <Button size="sm" variant="primary" disabled={busy} onclick={onresolve}>
        {msg('cloudsaves.resolve')}
      </Button>
    {/if}
    {#if game && game.state !== 'conflict' && game.code !== 'cloud.excluded'}
      <Button size="sm" disabled={busy || running} onclick={onpull}>
        {msg('cloudsaves.pull')}
      </Button>
    {/if}
    <Button size="sm" variant="danger" disabled={busy} onclick={ondelete}>
      {msg('cloudsaves.deleteRemote')}
    </Button>
  </div>
{/if}

<style>
  .hint {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .cloud-state {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .snapshot-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }
</style>
