<script lang="ts">
  import { ArrowDown, ArrowUp, Trash2, X } from '@lucide/svelte';
  import Button from '../Button.svelte';
  import IconButton from '../IconButton.svelte';
  import Select from '../Select.svelte';
  import { msg } from '../../i18n';
  import { layoutErrorText, runeCount, type LayoutError } from '../../profile/layout';
  import { showcaseTitle } from '../../profile/layoutView';
  import {
    COLLECTION_SOURCES,
    MAX_BLOCK_TITLE,
    MAX_CAPTION,
    MAX_COLLECTION_GAMES,
    MAX_TEXT_BODY,
    type ProfileBlock,
    type ProfileLayout,
  } from '../../services/account';
  import type { GameRef } from '../../services/profile';
  import GamePicker from './GamePicker.svelte';
  import { blockLabel } from './blockLabels';

  let {
    block,
    layout,
    titleOf,
    issue = null,
    disabled = false,
    onconfig,
    onremember,
    onremove,
    onclose,
  }: {
    block: ProfileBlock;
    layout: ProfileLayout;
    titleOf: (igdbId: number) => string;
    issue?: LayoutError | null;
    disabled?: boolean;
    onconfig: (patch: Record<string, unknown>) => void;
    onremember: (pick: { igdbId: number; game: GameRef }) => void;
    onremove: () => void;
    onclose: () => void;
  } = $props();

  const config = $derived(block.config);
  const name = $derived(blockLabel(block.type));
  const charError = (field: string) => (issue?.code === 'bad_chars' && issue.field === field ? layoutErrorText(issue) : '');
  const text = (key: string) => (typeof config[key] === 'string' ? (config[key] as string) : '');
  const igdbId = $derived(typeof config.igdbId === 'number' ? config.igdbId : 0);
  const source = $derived(text('source'));
  const ids = $derived(
    Array.isArray(config.igdbIds) ? (config.igdbIds as unknown[]).filter((id): id is number => typeof id === 'number') : [],
  );

  const sources = $derived(
    COLLECTION_SOURCES.filter(
      (item) =>
        item === 'manual' ||
        item === source ||
        !layout.blocks.some(
          (other) => other.id !== block.id && other.type === 'collection' && other.config.source === item,
        ),
    ).map((item) => ({ id: item, label: item === 'manual' ? msg('profile.sourceManual') : showcaseTitle(item) })),
  );

  function changeSource(next: string) {
    if (next === 'manual') onconfig({ source: next, title: '', igdbIds: [] });
    else onconfig({ source: next, title: undefined, igdbIds: undefined });
  }

  function shift(index: number, delta: number) {
    const next = [...ids];
    [next[index], next[index + delta]] = [next[index + delta], next[index]];
    onconfig({ igdbIds: next });
  }

  function pickPinned(pick: { igdbId: number; game: GameRef }) {
    onremember(pick);
    onconfig({ igdbId: pick.igdbId });
  }

  function pickCollection(pick: { igdbId: number; game: GameRef }) {
    onremember(pick);
    onconfig({ igdbIds: [...ids, pick.igdbId] });
  }
</script>

<section class="settings" aria-label={msg('profile.blockSettings', { name })}>
  <div class="head">
    <h2>{name}</h2>
    <IconButton label={msg('common.close')} onclick={onclose}><X size="1.8rem" /></IconButton>
  </div>
  <div class="scroll">
    <fieldset {disabled}>
      {#if block.type === 'pinned'}
        <div class="field">
          <span class="field-label">{msg('profile.pinnedGame')}</span>
          <p class="current">{igdbId > 0 ? titleOf(igdbId) : msg('profile.pinnedNone')}</p>
          <GamePicker chosen={igdbId > 0 ? [igdbId] : []} {disabled} onpick={pickPinned} />
        </div>
        <label class="field">
          <span class="field-label">
            {msg('profile.pinnedCaption')}
            <span class="counter">{runeCount(text('caption'))}/{MAX_CAPTION}</span>
          </span>
          <textarea
            class="input area"
            rows="3"
            maxlength={MAX_CAPTION}
            value={text('caption')}
            oninput={(event) => onconfig({ caption: event.currentTarget.value })}
          ></textarea>
          {#if charError('caption')}<span class="error" role="alert">{charError('caption')}</span>{/if}
        </label>
      {:else if block.type === 'collection'}
        <div class="field">
          <span class="field-label">{msg('profile.collectionSource')}</span>
          <Select options={sources} value={source} width="100%" onchange={changeSource} />
        </div>
        {#if source === 'manual'}
          <label class="field">
            <span class="field-label">
              {msg('profile.collectionTitle')}
              <span class="counter">{runeCount(text('title'))}/{MAX_BLOCK_TITLE}</span>
            </span>
            <input
              class="input"
              type="text"
              maxlength={MAX_BLOCK_TITLE}
              value={text('title')}
              oninput={(event) => onconfig({ title: event.currentTarget.value })}
            />
            {#if charError('title')}<span class="error" role="alert">{charError('title')}</span>{/if}
          </label>
          <div class="field">
            <span class="field-label">
              {msg('profile.collectionGames')}
              <span class="counter">{ids.length}/{MAX_COLLECTION_GAMES}</span>
            </span>
            {#if ids.length === 0}
              <p class="hint">{msg('profile.collectionEmpty')}</p>
            {:else}
              <ul class="games">
                {#each ids as id, index (id)}
                  <li>
                    <span class="game">{titleOf(id)}</span>
                    <IconButton size="sm" label={msg('social.moveUp', { title: titleOf(id) })} disabled={index === 0 || disabled} onclick={() => shift(index, -1)}>
                      <ArrowUp size="1.4rem" />
                    </IconButton>
                    <IconButton size="sm" label={msg('social.moveDown', { title: titleOf(id) })} disabled={index === ids.length - 1 || disabled} onclick={() => shift(index, 1)}>
                      <ArrowDown size="1.4rem" />
                    </IconButton>
                    <IconButton size="sm" label={msg('profile.collectionRemoveGame', { title: titleOf(id) })} {disabled} onclick={() => onconfig({ igdbIds: ids.filter((item) => item !== id) })}>
                      <X size="1.4rem" />
                    </IconButton>
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
          {#if ids.length < MAX_COLLECTION_GAMES}
            <div class="field">
              <span class="field-label">{msg('profile.collectionAdd')}</span>
              <GamePicker chosen={ids} {disabled} onpick={pickCollection} />
            </div>
          {:else}
            <p class="hint">{msg('profile.collectionFull', { count: MAX_COLLECTION_GAMES })}</p>
          {/if}
        {:else}
          <p class="hint">{msg('profile.collectionAutoHint')}</p>
        {/if}
      {:else if block.type === 'text'}
        <label class="field">
          <span class="field-label">
            {msg('profile.textTitle')}
            <span class="counter">{runeCount(text('title'))}/{MAX_BLOCK_TITLE}</span>
          </span>
          <input
            class="input"
            type="text"
            maxlength={MAX_BLOCK_TITLE}
            value={text('title')}
            oninput={(event) => onconfig({ title: event.currentTarget.value })}
          />
          {#if charError('title')}<span class="error" role="alert">{charError('title')}</span>{/if}
        </label>
        <label class="field">
          <span class="field-label">
            {msg('profile.textBody')}
            <span class="counter">{runeCount(text('body'))}/{MAX_TEXT_BODY}</span>
          </span>
          <textarea
            class="input area"
            rows="8"
            maxlength={MAX_TEXT_BODY}
            value={text('body')}
            oninput={(event) => onconfig({ body: event.currentTarget.value })}
          ></textarea>
          {#if charError('body')}<span class="error" role="alert">{charError('body')}</span>{/if}
        </label>
      {:else}
        <p class="hint">{msg('profile.blockNoSettings')}</p>
      {/if}
    </fieldset>
  </div>
  <div class="foot">
    <Button variant="ghost" {disabled} onclick={onremove}>
      <Trash2 size="1.4rem" />{msg('profile.removeBlock', { name })}
    </Button>
  </div>
</section>

<style>
  .settings {
    display: flex;
    flex-direction: column;
    max-height: calc(100dvh - 12rem);
    padding: 1.8rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  h2 {
    font-size: var(--font-lg);
  }

  .scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overscroll-behavior-y: contain;
    scrollbar-gutter: stable;
    padding: 0.3rem;
    margin: var(--space-3) -0.3rem 0;
  }

  fieldset {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
    margin: 0;
    padding: 0;
    border: 0;
  }

  fieldset:disabled {
    opacity: 0.65;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
  }

  .field-label {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.8rem;
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .counter {
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .input {
    width: 100%;
    height: var(--control-md);
    padding: 0 1.2rem;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    color: var(--text);
    font-size: var(--font-sm);
    font-family: inherit;
    transition:
      border-color var(--dur) var(--ease),
      box-shadow var(--dur) var(--ease);
  }

  .input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-subtle);
  }

  .area {
    height: auto;
    padding: 0.9rem 1.2rem;
    resize: vertical;
    line-height: 1.5;
  }

  .current {
    margin: 0;
    font-size: var(--font-sm);
    font-weight: 500;
    overflow-wrap: anywhere;
  }

  .games {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .games li {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    font-size: var(--font-sm);
  }

  .game {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .error {
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--danger);
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }

  .foot {
    flex-shrink: 0;
    padding-top: var(--space-3);
  }
</style>
