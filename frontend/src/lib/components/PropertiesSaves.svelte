<script lang="ts">
  import { FolderOpen } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import { msg } from '../i18n';
  import type { LibraryGame } from '../services/library';

  // The saves folder section of GamePropertiesModal, which holds its state and does its writes.
  let {
    game,
    savesBusy,
    savesError,
    onbrowse,
    onreset,
  }: {
    game: LibraryGame;
    savesBusy: '' | 'browse' | 'reset';
    savesError: string;
    onbrowse: () => void;
    onreset: () => void;
  } = $props();
</script>

<section class="section">
  <h4>{msg('modals.propertiesSavesTitle')}</h4>
  <p class="path">{game.savesDir || msg('modals.propertiesSavesAuto')}</p>
  {#if savesError}
    <Notice tone="danger" compact announce={false}>{savesError}</Notice>
  {/if}
  <div class="row">
    <Button size="sm" disabled={savesBusy === 'reset'} busy={savesBusy === 'browse'} onclick={onbrowse}>
      <FolderOpen size="1.5rem" strokeWidth={1.8} />
      {msg('modals.propertiesSavesBrowse')}
    </Button>
    {#if game.savesDir}
      <Button size="sm" variant="ghost" disabled={savesBusy === 'browse'} busy={savesBusy === 'reset'} onclick={onreset}>
        {msg('modals.propertiesSavesReset')}
      </Button>
    {/if}
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

  .path {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
</style>
