<script lang="ts">
  import { Power } from '@lucide/svelte';
  import Button from '../lib/components/Button.svelte';
  import { msg } from '../lib/i18n';
  import { libraryErrorText } from '../lib/library/errors';
  import { getGames, getRunningGames, stopGame } from '../lib/services/library';
  import { hideOverlay } from '../lib/services/overlay';
  import { runningRows, type RunningRow } from './data';

  let { active }: { active: boolean } = $props();

  let rows = $state<RunningRow[]>([]);
  let confirming = $state('');
  let closing = $state('');
  let failure = $state('');

  async function refresh(): Promise<void> {
    try {
      const [ids, games] = await Promise.all([getRunningGames(), getGames()]);
      rows = runningRows(ids, games);
    } catch (err) {
      console.error('overlay running games', err);
      rows = [];
    }
  }

  $effect(() => {
    if (!active) return;
    confirming = '';
    failure = '';
    void refresh();
  });

  async function close(id: string): Promise<void> {
    if (closing) return;
    closing = id;
    failure = '';
    try {
      await stopGame(id);
    } catch (err) {
      failure = libraryErrorText(err, msg('overlay.closeGameFailed'));
      return;
    } finally {
      closing = '';
    }
    confirming = '';
    await refresh();
    try {
      await hideOverlay();
    } catch (err) {
      // The game is closed and its row is gone, so there is nowhere to show
      // this; the overlay stays open for the player to dismiss.
      console.error('hide overlay after closing the game', err);
    }
  }
</script>

{#each rows as row (row.id)}
  <section class="game" aria-label={msg('overlay.gameLabel')}>
    <div class="line">
      <div class="info">
        <span>{msg('overlay.gameRunning')}</span>
        <strong title={row.title}>{row.title || msg('overlay.gameUntitled')}</strong>
      </div>
      {#if confirming !== row.id}
        <Button size="sm" onclick={() => (confirming = row.id)}>
          <Power size="1.54rem" strokeWidth={1.8} />
          {msg('overlay.closeGame')}
        </Button>
      {/if}
    </div>
    {#if confirming === row.id}
      <p class="note">{msg('overlay.closeGameNote')}</p>
      <div class="confirm">
        <Button size="sm" disabled={closing === row.id} onclick={() => (confirming = '')}>{msg('common.cancel')}</Button>
        <Button size="sm" variant="danger" disabled={closing === row.id} onclick={() => close(row.id)}>
          {closing === row.id ? msg('overlay.closingGame') : msg('overlay.closeGameConfirm')}
        </Button>
      </div>
    {/if}
    {#if failure && confirming === row.id}<p class="error" role="alert">{failure}</p>{/if}
  </section>
{/each}

<style>
  .game {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0 var(--space-4) var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .info {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    gap: 0.11rem;
  }

  .info span {
    color: var(--text-3);
    font-size: var(--font-xs);
  }

  .info strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--font-sm);
    font-weight: 600;
  }

  .note {
    margin: 0;
    color: var(--text-2);
    font-size: var(--font-xs);
    line-height: 1.5;
  }

  .confirm {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .error {
    margin: 0;
    color: var(--danger);
    font-size: var(--font-xs);
  }
</style>
