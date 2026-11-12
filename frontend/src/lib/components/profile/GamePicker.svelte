<script lang="ts">
  import { untrack } from 'svelte';
  import EmptyState from '../EmptyState.svelte';
  import Notice from '../Notice.svelte';
  import SearchInput from '../SearchInput.svelte';
  import Spinner from '../Spinner.svelte';
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
  let retrying = $state(false);
  let root = $state<HTMLElement>();
  let list = $state<HTMLElement>();
  let run = 0;
  let latest: Promise<boolean> = Promise.resolve(true);

  const candidates = $derived($libraryGames.filter((game) => Boolean(game.canonicalGameId) && !game.hidden));
  const needle = $derived(query.trim().toLowerCase());
  const rows = $derived(
    candidates.filter((game) => !needle || game.title.toLowerCase().includes(needle)).slice(0, LIMIT),
  );

  const idsKey = $derived(JSON.stringify(candidates.map((game) => game.canonicalGameId)));

  // Whether the load worked. A newer load (the library changed) replaces an older one's answer.
  function load(): Promise<boolean> {
    const ids = JSON.parse(idsKey) as string[];
    const current = ++run;
    loading = true;
    latest = igdbIdsOf(ids)
      .then((found) => {
        if (current === run) {
          mapping = found;
          failure = '';
        }
        return true;
      })
      .catch((err) => {
        if (current === run) failure = accountErrorText(err, msg('profile.pickerFailed'));
        return false;
      })
      .finally(() => {
        if (current === run) loading = false;
      });
    return latest;
  }

  $effect(() => {
    void idsKey;
    void untrack(load);
  });

  // The retry answers for the newest load, whichever started while it ran.
  async function retry(): Promise<boolean> {
    let pending = load();
    let ok = await pending;
    while (pending !== latest) {
      pending = latest;
      ok = await pending;
    }
    return ok;
  }

  function igdbOf(canonicalGameId: string | undefined): number {
    const value = Number(canonicalGameId ? mapping[canonicalGameId] : 0);
    return Number.isInteger(value) && value > 0 ? value : 0;
  }
</script>

<div class="picker" bind:this={root}>
  <SearchInput bind:value={query} label={msg('profile.pickerSearch')} placeholder={msg('profile.pickerSearch')} />
  {#if failure || retrying}
    <Notice tone="danger" compact bind:retrying retry={{ label: msg('common.retry'), run: retry, within: () => list ?? root }}>
      {failure}
    </Notice>
  {:else if loading}
    <Spinner size="sm" label={msg('social.loadingEllipsis')} />
  {:else if candidates.length === 0}
    <EmptyState size="inline" title={msg('profile.pickerEmpty')} />
  {:else if rows.length === 0}
    <EmptyState size="inline" title={msg('profile.pickerNoMatch')} />
  {:else}
    <ul class="list" bind:this={list}>
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

  /* Room inside the scrolling list for the shared focus ring around a row, which it would clip. */
  .list {
    list-style: none;
    margin: 0;
    padding: var(--space-1);
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
</style>
