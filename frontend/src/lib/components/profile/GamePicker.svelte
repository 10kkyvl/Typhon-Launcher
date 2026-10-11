<script lang="ts">
  import Button from '../Button.svelte';
  import SearchInput from '../SearchInput.svelte';
  import { msg } from '../../i18n';
  import { refOfLibrary } from '../../profile/layoutView';
  import { igdbIdsOf } from '../../services/profile';
  import type { GameRef } from '../../services/profile';
  import { accountErrorText } from '../../services/accountMessages';
  import { libraryGames } from '../../stores/library';

  let {
    chosen,
    disabled = false,
    onpick,
  }: {
    chosen: number[];
    disabled?: boolean;
    onpick: (pick: { igdbId: number; game: GameRef }) => void;
  } = $props();

  const LIMIT = 40;

  let query = $state('');
  let mapping = $state<Record<string, string>>({});
  let failure = $state('');
  let loading = $state(false);
  let attempt = $state(0);
  let run = 0;

  const candidates = $derived($libraryGames.filter((game) => Boolean(game.canonicalGameId)));
  const needle = $derived(query.trim().toLowerCase());
  const rows = $derived(
    candidates.filter((game) => !needle || game.title.toLowerCase().includes(needle)).slice(0, LIMIT),
  );

  const idsKey = $derived(JSON.stringify(candidates.map((game) => game.canonicalGameId)));

  $effect(() => {
    attempt;
    const ids = JSON.parse(idsKey) as string[];
    const current = ++run;
    loading = true;
    failure = '';
    igdbIdsOf(ids)
      .then((found) => {
        if (current === run) mapping = found;
      })
      .catch((err) => {
        if (current === run) failure = accountErrorText(err, msg('profile.pickerFailed'));
      })
      .finally(() => {
        if (current === run) loading = false;
      });
  });

  function igdbOf(canonicalGameId: string | undefined): number {
    const value = Number(canonicalGameId ? mapping[canonicalGameId] : 0);
    return Number.isInteger(value) && value > 0 ? value : 0;
  }
</script>

<div class="picker">
  <SearchInput bind:value={query} placeholder={msg('profile.pickerSearch')} />
  {#if failure}
    <p class="error" role="alert">{failure}</p>
    <div><Button size="sm" onclick={() => attempt++}>{msg('common.retry')}</Button></div>
  {:else if loading}
    <p class="hint">{msg('social.loadingEllipsis')}</p>
  {:else if candidates.length === 0}
    <p class="hint">{msg('profile.pickerEmpty')}</p>
  {:else if rows.length === 0}
    <p class="hint">{msg('profile.pickerNoMatch')}</p>
  {:else}
    <ul class="list">
      {#each rows as game (game.id)}
        {@const igdbId = igdbOf(game.canonicalGameId)}
        {@const taken = igdbId > 0 && chosen.includes(igdbId)}
        <li>
          <button
            class="row"
            type="button"
            disabled={disabled || igdbId === 0 || taken}
            onclick={() => onpick({ igdbId, game: refOfLibrary(game) })}
          >
            <span class="title">{game.title}</span>
            {#if taken}
              <span class="note">{msg('profile.pickerTaken')}</span>
            {:else if igdbId === 0}
              <span class="note">{msg('profile.pickerNoCatalog')}</span>
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 24rem;
    overflow-y: auto;
    overscroll-behavior-y: contain;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    width: 100%;
    padding: 0.8rem 1rem;
    border-radius: var(--radius-sm);
    background: none;
    color: inherit;
    font: inherit;
    font-size: var(--font-sm);
    text-align: left;
    cursor: pointer;
    transition: background var(--dur) var(--ease);
  }

  .row:hover:not(:disabled) {
    background: var(--hover-strong);
  }

  .row:disabled {
    cursor: default;
    color: var(--text-3);
  }

  .title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .note {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    line-height: 1.5;
  }

  .error {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--danger);
    line-height: 1.5;
  }
</style>
