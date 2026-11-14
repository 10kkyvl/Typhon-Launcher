<script lang="ts">
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import TextLink from './TextLink.svelte';
  import { saveAttribution } from '../game/extras';
  import { msg } from '../i18n';
  import type { SavesResult } from '../services/library';

  /**
   * Where one game's saves are, in the saves view: looking for the folder, the folder with where it was
   * found and Open / Change, several candidates to pick from, a failure, or nothing found. It draws what
   * the view holds and reports presses; the view runs the actions. `acting` names the folder action
   * that runs (its button stays focused and busy, the others wait while `busy`).
   */
  let {
    saves,
    loading = false,
    error = '',
    title,
    busy = false,
    acting = '',
    changeId,
    onopen,
    onchange,
    onpick,
    onsource,
  }: {
    saves: SavesResult | null;
    loading?: boolean;
    error?: string;
    /** The game's title, for the candidates line. */
    title: string;
    busy?: boolean;
    acting?: '' | 'open' | 'folder';
    /** The id of Change, where focus goes once a chosen folder replaces "Choose folder". */
    changeId?: string;
    onopen: () => void;
    onchange: () => void;
    onpick: () => void;
    onsource: () => void;
  } = $props();

  const attribution = $derived(saveAttribution(saves?.hint));
</script>

{#snippet source()}
  {#if attribution}
    <p class="source">
      {msg('saves.hintPrefix')}
      <TextLink disabled={busy} onclick={onsource}>{attribution.source}</TextLink>
      {attribution.via
        ? msg('saves.hintSuffix', { via: attribution.via, license: attribution.license })
        : msg('saves.hintSuffixNoVia', { license: attribution.license })}
    </p>
  {/if}
{/snippet}

<div class="field">
  <span class="field-label">{msg('saves.folderLabel')}</span>
  {#if loading && !saves && !error}
    <Spinner size="sm" label={msg('saves.folderLoading')} />
  {:else if saves?.path}
    <p class="path">{saves.path}</p>
    {@render source()}
    <div class="actions">
      <Button size="sm" disabled={busy && acting !== 'open'} busy={acting === 'open'} onclick={onopen}>{msg('saves.open')}</Button>
      <Button id={changeId} size="sm" disabled={busy && acting !== 'folder'} busy={acting === 'folder'} onclick={onchange}>{msg('saves.change')}</Button>
    </div>
  {:else if error}
    <Notice tone="danger" compact>{error}</Notice>
    <div class="actions">
      <Button size="sm" disabled={busy && acting !== 'folder'} busy={acting === 'folder'} onclick={onchange}>{msg('saves.pickFolder')}</Button>
    </div>
  {:else if saves?.candidates && saves.candidates.length > 0}
    <p class="hint">{msg('ui.savesMultipleCandidates', { title })}</p>
    {@render source()}
    <div class="actions">
      <Button size="sm" disabled={busy} onclick={onpick}>
        {msg('saves.pickFolder')}
      </Button>
    </div>
  {:else}
    <p class="hint">{saves && saves.unreadable > 0 ? msg('ui.savesPartialUnreadable') : msg('ui.savesNotFound')}</p>
    {#if saves?.hintsError}
      <p class="hint">{msg('saves.hintsFailed')}</p>
    {/if}
    <div class="actions">
      <Button size="sm" disabled={busy && acting !== 'folder'} busy={acting === 'folder'} onclick={onchange}>{msg('saves.pickFolder')}</Button>
    </div>
  {/if}
</div>

<style>
  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .source {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .path {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .hint {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }
</style>
