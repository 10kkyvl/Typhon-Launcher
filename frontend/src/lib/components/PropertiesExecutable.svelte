<script lang="ts">
  import { FolderOpen } from '@lucide/svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Notice from './Notice.svelte';
  import OptionList from './OptionList.svelte';
  import Spinner from './Spinner.svelte';
  import { msg } from '../i18n';
  import type { Candidate } from '../services/install';
  import type { LibraryGame } from '../services/library';

  // The executable section of GamePropertiesModal, which holds its state and does its writes.
  let {
    game,
    emulated,
    candidates,
    candidatesLoading,
    candidatesError,
    exeBusy,
    exeError,
    onpick,
    onbrowse,
  }: {
    game: LibraryGame;
    emulated: boolean;
    candidates: Candidate[];
    candidatesLoading: boolean;
    candidatesError: string;
    exeBusy: '' | 'pick' | 'browse';
    exeError: string;
    onpick: (path: string) => void;
    onbrowse: () => void;
  } = $props();
  const uid = $props.id();

  // A file is set as it is chosen, so the arrows only walk the list. The path wraps where it must:
  // nothing is cut.
  const options = $derived(candidates.map((candidate) => ({ id: candidate.path, title: candidate.path })));
  const held = $derived(exeBusy !== '' ? msg('modals.actionRunning') : '');
</script>

<section class="section">
  <h4>{msg('modals.propertiesExeTitle')}</h4>
  <p class="path">
    {game.executable ? msg('modals.propertiesExeCurrent', { path: game.executable }) : msg('modals.propertiesExeNone')}
  </p>
  {#if emulated}
    <p class="hint">{msg('emul.imageHint')}</p>
  {:else if candidatesLoading}
    <Spinner size="sm" label={msg('modals.propertiesExeLoading')} />
  {:else if candidatesError}
    <Notice tone="danger" compact>{candidatesError}</Notice>
  {:else if candidates.length === 0}
    <EmptyState size="inline" title={msg('modals.propertiesExeNoCandidates')} />
  {:else}
    <span class="label" id="{uid}-candidates">{msg('modals.propertiesExeCandidates')}</span>
    <OptionList
      labelledby="{uid}-candidates"
      describedby={held ? `${uid}-held` : undefined}
      {options}
      value={game.executable}
      activation="manual"
      disabled={exeBusy !== ''}
      onchange={onpick}
    />
  {/if}
  {#if held}
    <p class="hint" id="{uid}-held">{held}</p>
  {/if}
  {#if exeError}
    <Notice tone="danger" compact>{exeError}</Notice>
  {/if}
  <div>
    <Button size="sm" disabled={exeBusy === 'pick'} reason={exeBusy === 'pick' ? held : ''} busy={exeBusy === 'browse'} onclick={onbrowse}>
      <FolderOpen size="1.5rem" strokeWidth={1.8} />
      {msg('modals.propertiesExeBrowse')}
    </Button>
  </div>
</section>

<style>
  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h4 {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text);
  }

  .label {
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
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
