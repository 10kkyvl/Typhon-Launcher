<script lang="ts">
  import { untrack } from 'svelte';
  import { ArrowDown, ArrowUp, Trash2, X } from '@lucide/svelte';
  import Button from '../Button.svelte';
  import EmptyState from '../EmptyState.svelte';
  import IconButton from '../IconButton.svelte';
  import Select from '../Select.svelte';
  import TextArea from '../TextArea.svelte';
  import TextField from '../TextField.svelte';
  import type { TextAreaCounter } from '../textArea';
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
  import { blockPlace, blockRows, gameRows } from './rowFocus';
  import { focusHandOff } from '../../utils/focusHandOff';

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

  const uid = $props.id();
  const config = $derived(block.config);
  const name = $derived(blockLabel(block.type));
  const charError = (field: string) => (issue?.code === 'bad_chars' && issue.field === field ? layoutErrorText(issue) : '');
  const text = (key: string) => (typeof config[key] === 'string' ? (config[key] as string) : '');
  // While the draft saves, the settings refuse edits without turning disabled: a pointer press on Save
  // moves no focus in the macOS view, so focus may still be in here, and a disabled control drops it.
  const held = $derived(disabled ? msg('social.saving') : '');

  // Counted as the layout's own check counts its limits: in characters.
  function counter(max: number): TextAreaCounter {
    return {
      max,
      measure: runeCount,
      text: (count, limit) => msg('profile.fieldCounter', { count, max: limit }),
      limitText: (count, limit) => {
        if (count > limit) return msg('profile.fieldOver', { count: count - limit });
        if (count === limit) return msg('profile.fieldAt');
        return msg('profile.fieldNear', { count: limit - count });
      },
    };
  }

  // A one-line field has no counter of its own: its count stands beside its name, as a text area's
  // does, and stays out of its description, which is its error alone.
  const counted = (key: string, max: number) => msg('profile.fieldCounter', { count: runeCount(text(key)), max });
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

  // A game moved or removed while focus is on one of its buttons: focus stays on that button, or goes
  // to the same button of the game now in its place.
  $effect.pre(() => {
    void ids;
    untrack(gameRows.catchLeaving);
  });

  // The settings close with their block: focus goes to the settings button of the block now in its
  // place in the grid, else of the one above, else to the control after the grid.
  function removeBlock() {
    const place = blockPlace(block.id, 'settings');
    const handOff = focusHandOff();
    onremove();
    void handOff(() => blockRows.leaveTarget(place));
  }

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

{#snippet titleField(label: string)}
  <div class="field">
    <div class="field-label">
      <label id="{uid}-title-label" for="{uid}-title">{label}</label>
      <span class="counter">{counted('title', MAX_BLOCK_TITLE)}</span>
    </div>
    <TextField
      id="{uid}-title"
      labelledby="{uid}-title-label"
      maxlength={MAX_BLOCK_TITLE}
      readonly={disabled}
      error={charError('title')}
      value={text('title')}
      oninput={(event) => onconfig({ title: event.currentTarget.value })}
    />
  </div>
{/snippet}

<section class="settings" aria-label={msg('profile.blockSettings', { name })}>
  <div class="head">
    <h2>{name}</h2>
    <IconButton label={msg('common.close')} onclick={onclose}><X size="1.8rem" /></IconButton>
  </div>
  <div class="scroll">
    <div class="fields">
      {#if block.type === 'pinned'}
        <div class="field">
          <span class="field-label">{msg('profile.pinnedGame')}</span>
          <p class="current">{igdbId > 0 ? titleOf(igdbId) : msg('profile.pinnedNone')}</p>
          <GamePicker chosen={igdbId > 0 ? [igdbId] : []} {disabled} onpick={pickPinned} />
        </div>
        <TextArea
          label={msg('profile.pinnedCaption')}
          rows={3}
          maxlength={MAX_CAPTION}
          counter={counter(MAX_CAPTION)}
          readonly={disabled}
          error={charError('caption')}
          value={text('caption')}
          oninput={(event) => onconfig({ caption: event.currentTarget.value })}
        />
      {:else if block.type === 'collection'}
        <div class="field">
          <span class="field-label" id="{uid}-source-label">{msg('profile.collectionSource')}</span>
          <Select options={sources} value={source} width="100%" labelledby="{uid}-source-label" {disabled} onchange={changeSource} />
        </div>
        {#if source === 'manual'}
          {@render titleField(msg('profile.collectionTitle'))}
          <div class="field">
            <span class="field-label">
              {msg('profile.collectionGames')}
              <span class="counter">{msg('profile.fieldCounter', { count: ids.length, max: MAX_COLLECTION_GAMES })}</span>
            </span>
            {#if ids.length === 0}
              <EmptyState size="inline" title={msg('profile.collectionEmpty')} />
            {:else}
              <ul class="games">
                {#each ids as id, index (id)}
                  <li data-collection-game={id}>
                    <span class="game">{titleOf(id)}</span>
                    <IconButton
                      size="sm"
                      label={msg('social.moveUp', { title: titleOf(id) })}
                      data-row-control="up"
                      disabled={index === 0 || disabled}
                      reason={held || (index === 0 ? msg('profile.gameFirst') : '')}
                      onclick={() => shift(index, -1)}
                    >
                      <ArrowUp size="1.4rem" />
                    </IconButton>
                    <IconButton
                      size="sm"
                      label={msg('social.moveDown', { title: titleOf(id) })}
                      data-row-control="down"
                      disabled={index === ids.length - 1 || disabled}
                      reason={held || (index === ids.length - 1 ? msg('profile.gameLast') : '')}
                      onclick={() => shift(index, 1)}
                    >
                      <ArrowDown size="1.4rem" />
                    </IconButton>
                    <IconButton
                      size="sm"
                      label={msg('profile.collectionRemoveGame', { title: titleOf(id) })}
                      data-row-control="remove"
                      {disabled}
                      reason={held}
                      onclick={() => onconfig({ igdbIds: ids.filter((item) => item !== id) })}
                    >
                      <X size="1.4rem" />
                    </IconButton>
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
          {#if ids.length < MAX_COLLECTION_GAMES}
            <div class="field" data-collection-add>
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
        {@render titleField(msg('profile.textTitle'))}
        <TextArea
          label={msg('profile.textBody')}
          rows={8}
          maxlength={MAX_TEXT_BODY}
          counter={counter(MAX_TEXT_BODY)}
          readonly={disabled}
          error={charError('body')}
          value={text('body')}
          oninput={(event) => onconfig({ body: event.currentTarget.value })}
        />
      {:else}
        <p class="hint">{msg('profile.blockNoSettings')}</p>
        {#if block.type === 'completed'}
          <p class="hint">{msg('profile.completedVisibility')}</p>
        {/if}
      {/if}
    </div>
  </div>
  <div class="foot">
    <Button variant="ghost" {disabled} reason={held} onclick={removeBlock}>
      <Trash2 size="1.4rem" />{msg('profile.removeBlock', { name })}
    </Button>
  </div>
</section>

<style>
  .settings {
    display: flex;
    flex-direction: column;
    max-height: calc(100dvh - var(--titlebar-h) - 12rem);
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

  .fields {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
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
