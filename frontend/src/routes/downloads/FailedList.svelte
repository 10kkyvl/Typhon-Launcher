<script lang="ts">
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { CircleAlert, RotateCcw, X } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import { installErrorText } from '../../lib/install/installErrors';
  import { gamePageId, type Download } from '../../lib/services/downloads';
  import { downloadOffers } from '../../lib/stores/downloadOffers';
  import { failed, requestDownloadAction } from '../../lib/stores/downloads';
  import { gameArt } from '../../lib/stores/metadata';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { bytesSize } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import DownloadsSection from './DownloadsSection.svelte';
  import { cancelDownloadPrompt } from '../../lib/confirm/prompts';
  import { cancelAsks } from '../../lib/downloads/cancel';
  import { coverOf } from './downloadRow';
  import { afterLeaving, byData, createWatchers, handOffInView, pageHeading, type HandOff } from './rowFocus';

  /**
   * Downloads Go marked failed, apart from the others: what failed (by its code), how much was
   * downloaded, Retry (Go's Resume), Details and Cancel (Go's Cancel, asked first when data exists).
   */
  let {
    onopen,
    onannounce = () => {},
  }: {
    onopen: (id: string) => void;
    /** Speaks through the screen's one polite live region: a retry, a cancel. */
    onannounce?: (text: string) => void;
  } = $props();

  let list = $state<HTMLElement>();
  const ids = $derived($failed.map((d) => d.id));

  const retryIn = (row: HTMLElement) => row.querySelector<HTMLElement>('[data-retry]') ?? cancelIn(row);
  const cancelIn = (row: HTMLElement) => row.querySelector<HTMLElement>('[data-cancel]');

  const watchers = createWatchers();
  $effect(() => {
    void ids;
    untrack(() => watchers.check());
  });

  const gone = (id: string) => () => !ids.includes(id);
  const busyRetry = new SvelteSet<string>();
  const busyCancel = new SvelteSet<string>();

  // A retried download goes back to the queue (or straight to verifying). Focus stays in this list while
  // it has failures to work through, as after a cancel; with none left it follows the download to its
  // grip in the queue, and otherwise to the page heading: the active panel is not this list's to know.
  async function retry(download: Download, index: number) {
    const handOff = focusHandOff();
    busyRetry.add(download.id);
    const outcome = await requestDownloadAction(download.id, 'resume');
    busyRetry.delete(download.id);
    if (!outcome.ok) return;
    onannounce(msg('downloads.failed.retried', { title: download.name }));
    watchers.watch(gone(download.id), () =>
      void handOffInView(handOff, () => {
        const near = afterLeaving(list, download.id, index, retryIn);
        if (near && near !== pageHeading()) return near;
        return byData(document, 'grip', download.id) ?? near;
      }),
    );
  }

  let asking = $state<{ download: Download; index: number; handOff: HandOff } | null>(null);

  function cancelled(download: Download, index: number, handOff: HandOff) {
    onannounce(msg('downloads.cancel.done', { name: download.name }));
    watchers.watch(gone(download.id), () => void handOffInView(handOff, () => afterLeaving(list, download.id, index, cancelIn)));
  }

  async function askCancel(download: Download, index: number) {
    const handOff = focusHandOff();
    if (cancelAsks(download)) {
      asking = { download, index, handOff };
      return;
    }
    busyCancel.add(download.id);
    const outcome = await requestDownloadAction(download.id, 'cancel');
    busyCancel.delete(download.id);
    if (outcome.ok) cancelled(download, index, handOff);
  }

  async function confirmCancel() {
    const current = asking;
    if (!current) return;
    const outcome = await requestDownloadAction(current.download.id, 'cancel', { toast: false });
    if (!outcome.ok) throw outcome.error;
    cancelled(current.download, current.index, current.handOff);
  }
</script>

{#if $failed.length > 0}
<DownloadsSection title={msg('transfers.downloadsFailedHeading')} count={$failed.length}>
  <ul class="rows" bind:this={list}>
    {#each $failed as item, index (item.id)}
      {@const offers = $downloadOffers.get(item.id) ?? []}
      {@const gameId = gamePageId(item)}
      <li class="row failed" data-id={item.id}>
        <div class="thumb">
          <GameLink id={gameId} name={item.name} repeat>
            <Artwork src={coverOf(item, $gameArt)} alt="" label={item.name} ratio="3 / 4" radius="var(--radius-sm)" />
          </GameLink>
        </div>
        <div class="info">
          <span class="title wraps"><GameLink id={gameId} name={item.name} /></span>
          <span class="error-text">
            <CircleAlert size="1.4rem" strokeWidth={1.8} aria-hidden="true" />
            <span>{installErrorText(item.error)}</span>
          </span>
          <span class="amount">
            {item.downloaded > 0
              ? msg('downloads.failed.downloaded', { done: bytesSize(item.downloaded), total: bytesSize(item.total) })
              : msg('downloads.failed.nothing')}
          </span>
        </div>
        <div class="row-actions">
          {#if offers.includes('resume')}
            <Button
              size="sm"
              busy={busyRetry.has(item.id)}
              aria-label={msg('downloads.failed.retryFor', { title: item.name })}
              data-retry={item.id}
              onclick={() => retry(item, index)}
            >
              <RotateCcw size="1.4rem" strokeWidth={1.8} aria-hidden="true" />
              {msg('common.retry')}
            </Button>
          {/if}
          <Button size="sm" variant="ghost" aria-label={msg('downloads.failed.detailsFor', { title: item.name })} onclick={() => onopen(item.id)}>
            {msg('downloads.queue.details')}
          </Button>
          {#if offers.includes('cancel')}
            <IconButton
              label={msg('downloads.queue.cancelFor', { title: item.name })}
              size="sm"
              busy={busyCancel.has(item.id)}
              data-cancel={item.id}
              onclick={() => askCancel(item, index)}
            >
              <X size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          {/if}
        </div>
      </li>
    {/each}
  </ul>
</DownloadsSection>
{/if}

{#if asking}
  {@const current = asking}
  <ConfirmModal
    prompt={cancelDownloadPrompt(current.download)}
    onconfirm={confirmCancel}
    errorText={installErrorText}
    onclose={() => (asking = null)}
    returnFocus={() => afterLeaving(list, current.download.id, current.index, cancelIn)}
  />
{/if}

<style>
  .rows {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    min-height: 6.4rem;
    padding: var(--space-2) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .row.failed {
    border-color: color-mix(in srgb, var(--danger) 35%, var(--border));
  }

  .error-text {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    max-width: 100%;
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .error-text span {
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .error-text :global(svg) {
    flex-shrink: 0;
  }

  .amount {
    font-size: var(--font-xs);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .thumb {
    width: 3.6rem;
    flex-shrink: 0;
    overflow: hidden;
    border-radius: var(--radius-sm);
  }

  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-1);
  }

  .title {
    min-width: 0;
    max-width: 100%;
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  /* A name is shown whole, wrapping in its row: an ellipsis would hide all of a text link (a button,
     an atomic box), and a hint on the rest would reach a mouse only. */
  .title.wraps {
    white-space: normal;
    overflow: visible;
    overflow-wrap: anywhere;
  }

  .row-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-2);
    flex-shrink: 0;
  }
</style>
