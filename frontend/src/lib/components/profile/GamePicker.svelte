<script lang="ts">
  import { untrack } from 'svelte';
  import EmptyState from '../EmptyState.svelte';
  import Notice from '../Notice.svelte';
  import SearchInput from '../SearchInput.svelte';
  import Spinner from '../Spinner.svelte';
  import { msg } from '../../i18n';
  import { focusableIn } from '../../layers/focus';
  import { libraryErrorText } from '../../library/errors';
  import { refOfLibrary } from '../../profile/layoutView';
  import { igdbIdsOf } from '../../services/profile';
  import type { GameRef } from '../../services/profile';
  import type { LibraryGame } from '../../services/library';
  import { accountErrorText } from '../../services/accountMessages';
  import { libraryGames, libraryLoaded, libraryLoadError, loadLibrary } from '../../stores/library';
  import { focusHandOff } from '../../utils/focusHandOff';

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
  let libraryRetrying = $state(false);
  let root = $state<HTMLElement>();
  let list = $state<HTMLElement>();
  let run = 0;
  let latest: Promise<boolean> = Promise.resolve(true);

  const uid = $props.id();
  const candidates = $derived($libraryGames.filter((game) => Boolean(game.canonicalGameId) && !game.hidden));
  // The library's first list did not arrive: an empty list then says nothing about the library.
  const libraryFailed = $derived(candidates.length === 0 && !$libraryLoaded && $libraryLoadError != null);
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

  /** The control just before the picker on the page: the one that opened it, in the support form. */
  function opener(): HTMLElement | null {
    const self = root;
    if (!self) return null;
    const before = focusableIn(document.body).filter((el) => !self.contains(el) && (self.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_PRECEDING) !== 0);
    return before.at(-1) ?? null;
  }

  // A row that cannot be picked stays a focusable button, held: a pressed row turns taken under the
  // focus it holds, and a disabled one would drop it.
  function pick(game: LibraryGame, igdbId: number, held: boolean) {
    if (held) return;
    // A pick the page answers by closing the picker (the support form) hands focus back to what opened it.
    const back = opener();
    const handOff = focusHandOff();
    onpick({ igdbId, game: refOfLibrary(game) });
    void handOff(() => back);
  }

  // A held key repeats its keydown, and a button takes each Enter as a press: one press, one pick.
  function once(event: KeyboardEvent) {
    if (event.repeat && (event.key === 'Enter' || event.key === ' ')) event.preventDefault();
  }
</script>

<div class="picker" bind:this={root}>
  <SearchInput bind:value={query} label={msg('profile.pickerSearch')} placeholder={msg('profile.pickerSearch')} />
  {#if libraryFailed || libraryRetrying}
    <Notice tone="danger" compact bind:retrying={libraryRetrying} retry={{ label: msg('common.retry'), run: () => loadLibrary(), within: () => root }}>
      {libraryErrorText($libraryLoadError, msg('library.loadFailedText'))}
    </Notice>
  {:else if failure || retrying}
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
        {@const held = disabled || igdbId === 0 || taken}
        {@const noteId = `${uid}-${game.id}-note`}
        <li>
          <button
            class="row"
            type="button"
            aria-disabled={held ? 'true' : undefined}
            aria-describedby={taken || igdbId === 0 ? noteId : undefined}
            onkeydown={once}
            onclick={() => pick(game, igdbId, held)}
          >
            <span class="title">{game.title}</span>
            {#if taken}
              <span class="note" id={noteId}>{msg('profile.pickerTaken')}</span>
            {:else if igdbId === 0}
              <span class="note" id={noteId}>{msg('profile.pickerNoCatalog')}</span>
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

  .row:hover:not([aria-disabled='true']) {
    background: var(--hover-strong);
  }

  .row[aria-disabled='true'] {
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
