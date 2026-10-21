<script lang="ts">
  import { Lock, RefreshCw, Trophy } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import { errorCode, hasMessage, msg, t } from '../i18n';
  import {
    CODE_NO_STEAM_ID,
    fetchAchievements,
    rescanAchievements,
    type AchievementItem,
    type AchievementsView,
  } from '../services/achievements';
  import { achievementsStamp } from '../stores/achievements';
  import { toast } from '../stores/toasts';
  import { dateTime } from '../utils/format';
  import { errorMessage } from '../utils/errors';
  import Card from './Card.svelte';
  import ProgressBar from './ProgressBar.svelte';

  let { gameId }: { gameId: string } = $props();

  let view = $state<AchievementsView | null>(null);
  let refreshing = $state(false);
  let loadError = $state('');
  let noSteam = $state(false);
  let active = $state<AchievementItem | null>(null);
  let seq = 0;
  let loadedId = '';

  const stamp = $derived($achievementsStamp[gameId] ?? 0);
  const shown = $derived(
    !noSteam && (loadError !== '' || (view !== null && view.source === 'local' && view.total > 0)),
  );
  const namesMissing = $derived(view !== null && view.schemaError !== '');
  const schemaNote = $derived(
    view?.schemaError === 'none'
      ? $t('games.achievementsSchemaNone')
      : view?.schemaError === 'unavailable'
        ? $t('games.achievementsSchemaUnavailable')
        : $t('games.achievementsSchemaUnknown'),
  );

  function failureText(err: unknown, fallback: string): string {
    const code = errorCode(err);
    return hasMessage(code) ? msg(code) : `${fallback}: ${errorMessage(err)}`;
  }

  async function load(id: string) {
    const mine = ++seq;
    loadError = '';
    noSteam = false;
    if (id !== loadedId) view = null;
    loadedId = id;
    try {
      const next = await fetchAchievements(id);
      if (mine !== seq) return;
      view = next;
    } catch (err) {
      if (mine !== seq) return;
      view = null;
      if (errorCode(err) === CODE_NO_STEAM_ID) noSteam = true;
      else loadError = failureText(err, msg('games.achievementsLoadError'));
    }
  }

  async function refresh() {
    if (refreshing) return;
    const mine = ++seq;
    refreshing = true;
    try {
      const next = await rescanAchievements(gameId);
      if (mine !== seq) return;
      view = next;
      loadError = '';
    } catch (err) {
      if (mine !== seq) return;
      toast(failureText(err, msg('games.achievementsRefreshError')), 'danger');
    } finally {
      refreshing = false;
    }
  }

  $effect(() => {
    void stamp;
    const id = gameId;
    untrack(() => {
      active = null;
      void load(id);
    });
  });

  function label(item: AchievementItem): string {
    return item.name || item.key;
  }

  function concealed(item: AchievementItem): boolean {
    return item.hidden && !item.unlocked;
  }

  function unlockedOn(item: AchievementItem): string {
    if (!item.unlocked || item.unlockedAt === '') return '';
    const at = new Date(item.unlockedAt);
    if (Number.isNaN(at.getTime())) return '';
    return $t('games.achievementsUnlockedOn', { date: dateTime(at) });
  }

  function tileName(item: AchievementItem): string {
    return concealed(item) ? $t('games.achievementsHidden') : label(item);
  }

  function pick(item: AchievementItem) {
    active = item;
  }

  function drop(item: AchievementItem) {
    if (active?.key === item.key) active = null;
  }
</script>

{#if shown}
  <Card title={$t('games.achievementsTitle')} surface="panel" padding="var(--space-5)">
    {#snippet action()}
      {#if view}
        <button class="refresh" type="button" disabled={refreshing} onclick={refresh}>
          <RefreshCw size="1.4rem" strokeWidth={1.8} class={refreshing ? 'spin' : ''} />
          {$t('games.achievementsRefresh')}
        </button>
      {/if}
    {/snippet}

    {#if view}
      <div class="progress">
        <span class="count">{$t('games.achievementsProgress', { unlocked: view.unlocked, total: view.total })}</span>
        <ProgressBar value={view.unlocked} max={view.total} height={6} />
      </div>

      {#if namesMissing}
        <p class="note warn">{schemaNote}</p>
      {/if}
      {#if view.partial}
        <p class="note warn">{$t('games.achievementsPartial')}</p>
      {/if}

      <ul class="grid" aria-label={$t('games.achievementsListLabel')}>
        {#each view.items as item (item.key)}
          <li>
            <button
              type="button"
              class="tile"
              class:locked={!item.unlocked}
              class:concealed={concealed(item)}
              class:selected={active?.key === item.key}
              aria-label={tileName(item)}
              onmouseenter={() => pick(item)}
              onmouseleave={() => drop(item)}
              onfocus={() => pick(item)}
              onblur={() => drop(item)}
            >
              {#if concealed(item)}
                <Lock size="1.8rem" strokeWidth={1.8} />
              {:else if (item.unlocked ? item.icon : item.iconGray || item.icon) !== ''}
                <img
                  src={item.unlocked ? item.icon : item.iconGray || item.icon}
                  alt=""
                  loading="lazy"
                  decoding="async"
                  draggable="false"
                />
              {:else}
                <Trophy size="1.8rem" strokeWidth={1.8} />
              {/if}
            </button>
          </li>
        {/each}
      </ul>

      <div class="detail" aria-live="polite">
        {#if active}
          {#if concealed(active)}
            <strong class="name">{$t('games.achievementsHidden')}</strong>
            <span class="desc">{$t('games.achievementsLocked')}</span>
          {:else}
            <strong class="name">{label(active)}</strong>
            {#if active.description}
              <span class="desc">{active.description}</span>
            {/if}
            {#if unlockedOn(active)}
              <span class="date">{unlockedOn(active)}</span>
            {/if}
          {/if}
        {/if}
      </div>

      <p class="source">{$t('games.achievementsSource')}</p>
    {:else if loadError}
      <p class="state error">{loadError}</p>
      <button class="refresh" type="button" onclick={() => load(gameId)}>
        <RefreshCw size="1.4rem" strokeWidth={1.8} />
        {$t('common.retry')}
      </button>
    {/if}
  </Card>
{/if}

<style>
  .refresh {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-sm);
    color: var(--accent-text);
    font-size: var(--font-xs);
  }

  .refresh:hover:not(:disabled) {
    background: var(--hover);
  }

  .refresh:disabled {
    color: var(--text-3);
    cursor: default;
  }

  .refresh :global(.spin) {
    animation: achievements-spin 1s linear infinite;
  }

  .progress {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin-bottom: var(--space-4);
  }

  .count {
    color: var(--text-2);
    font-size: var(--font-sm);
  }

  .note {
    margin-bottom: var(--space-3);
    font-size: var(--font-xs);
    line-height: 1.4;
  }

  .note.warn {
    color: var(--warning);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(4.4rem, 1fr));
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .tile {
    display: grid;
    place-items: center;
    width: 100%;
    aspect-ratio: 1;
    padding: 0;
    overflow: hidden;
    background: var(--surface-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text-2);
    cursor: default;
  }

  .tile img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .tile.locked {
    color: var(--text-3);
  }

  .tile.locked img {
    opacity: 0.55;
  }

  .tile:hover,
  .tile:focus-visible,
  .tile.selected {
    border-color: var(--accent);
  }

  .tile:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 1px;
  }

  .detail {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-height: 6.4rem;
    margin-top: var(--space-3);
  }

  .name {
    font-size: var(--font-sm);
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .desc,
  .date {
    color: var(--text-2);
    font-size: var(--font-xs);
    line-height: 1.4;
  }

  .date {
    color: var(--text-3);
  }

  .source {
    margin-top: var(--space-2);
    color: var(--text-3);
    font-size: var(--font-xs);
    line-height: 1.4;
  }

  .state {
    color: var(--text-3);
    font-size: var(--font-sm);
  }

  .state.error {
    margin-bottom: var(--space-2);
    color: var(--danger);
  }

  @keyframes achievements-spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
