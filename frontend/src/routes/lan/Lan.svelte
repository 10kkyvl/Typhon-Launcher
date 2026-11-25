<script lang="ts">
  import { untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import Page from '../../lib/components/Page.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import type { ConfirmPrompt } from '../../lib/confirm/prompts';
  import { Download, MonitorSmartphone, Share2, Wifi, X } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import SectionHeader from '../../lib/components/SectionHeader.svelte';
  import StatTile from '../../lib/components/StatTile.svelte';
  import { rejectedSummary, sharedGameTitle, transferLabel } from '../../lib/lan/lanText';
  import type { Offer, Transfer } from '../../lib/services/lan';
  import { libraryGames, libraryGamesById } from '../../lib/stores/library';
  import { navigate } from '../../lib/stores/router';
  import { settings } from '../../lib/stores/settings';
  import {
    cancel,
    cancelTransfer,
    hashing,
    lanLoaded,
    lanLoadError,
    lanStats,
    offers,
    receive,
    reloadLan,
    shares,
    transfers,
    unshare,
    watchOffers,
  } from '../../lib/stores/lan';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';
  import { bytesSize, formatCount, progressPercent, relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import { offerKey, offersByComputer } from './offerGroups';
  import { catchLeaving, placeOf, returnTo, type RowPlace } from './rowFocus';

  let retrying = $state(false);
  let content = $state<HTMLElement>();
  // The actions Go has not answered yet, by row: a second press waits for the first answer.
  const pending = new SvelteSet<string>();
  let asking = $state<{ transfer: Transfer; place: RowPlace | null } | null>(null);

  const uid = $props.id();
  const headings = { nearby: `${uid}-nearby`, sharing: `${uid}-sharing`, transfers: `${uid}-transfers` };

  // Go has no offers while sharing is off (lan.Service.Available answers none): nothing nearby can be
  // shown, and the screen says why instead. Settings not read yet count as not known, not as off.
  const sharingOff = $derived($settings?.lanSharing === false);

  const loadFailed = $derived(!$lanLoaded && $lanLoadError != null);
  const loading = $derived(!$lanLoaded && !loadFailed);

  // Go sends no event when the offers nearby change: they are read again while the screen shows them.
  $effect(() => watchOffers());

  const groups = $derived(offersByComputer($offers));

  const preparing = $derived(
    [...$hashing.values()].filter((p) => !$shares.some((s) => s.gameId === p.gameId && s.enabled)),
  );

  const activeTransfers = $derived($transfers.filter((t) => t.status === 'receiving'));

  // A transfer that ended while its cancel was asked (Go finished, failed or cancelled it) has nothing
  // left to cancel: its question goes with its row, and focus goes on by the row rule.
  const asked = $derived(asking && activeTransfers.some((t) => t.id === asking?.transfer.id) ? asking : null);

  const rejectedNote = $derived(rejectedSummary($lanStats));

  // Before any change of the rows shown (a share stopped, an offer Go dropped, the offers regrouped, a
  // transfer that ended), the screen's rule reads where focus is, and hands it on if the focused row went.
  $effect.pre(() => {
    void $offers;
    void $shares;
    void preparing;
    void activeTransfers;
    void $lanLoaded;
    void sharingOff;
    untrack(catchLeaving);
  });

  /** Runs a row's action once until Go answers; the store says a refusal in the words of its code. */
  async function act(key: string, action: () => Promise<void>) {
    if (pending.has(key)) return;
    pending.add(key);
    try {
      await action();
    } finally {
      pending.delete(key);
    }
  }

  // A cancel deletes what was received (Go's discardReceiving): it asks first when there is something,
  // as a download's cancel does (lib/downloads/cancel.ts).
  function askCancel(transfer: Transfer) {
    if (transfer.downloaded > 0) asking = { transfer, place: placeOf(document.activeElement) };
    else void act(`cancel:${transfer.id}`, () => cancel(transfer.id));
  }

  function cancelPrompt(transfer: Transfer): ConfirmPrompt {
    return {
      title: msg('transfers.lanCancelTitle', { title: transfer.title }),
      text: msg('transfers.lanCancelText', { size: bytesSize(transfer.downloaded) }),
      confirm: msg('transfers.lanCancelConfirm'),
      busy: msg('transfers.lanCancelBusy'),
      cancel: msg('ui.dontCancel'),
    };
  }

  /** The library's id of a game of this computer, for its link; none once the library no longer holds it. */
  const pageOf = (gameId: string) => $libraryGamesById.get(gameId)?.id;

  function transferProgress(transfer: Transfer): { value: number; text: string } | null {
    if (transfer.total <= 0) return null;
    const fraction = transfer.downloaded / transfer.total;
    const percent = progressPercent(fraction);
    return {
      value: Math.min(100, fraction * 100),
      text: msg('transfers.lanTransferProgress', { done: bytesSize(transfer.downloaded), total: bytesSize(transfer.total), percent }),
    };
  }
</script>

{#snippet offerRow(offer: Offer, grouped: boolean)}
  <li class="row" data-lan-row={offerKey(offer)}>
    <div class="main">
      <p class="title">{offer.title}</p>
      {#if !grouped}
        <p class="sub">
          <MonitorSmartphone size="1.3rem" strokeWidth={1.8} />
          {msg('transfers.lanFromHost', { host: offer.host })}
        </p>
      {/if}
    </div>
    {#if offer.version}<span class="fact version">{offer.version}</span>{/if}
    <span class="fact size">{bytesSize(offer.sizeBytes)}</span>
    <div class="actions">
      <Button
        size="sm"
        data-row-control="download"
        aria-label={msg('transfers.lanDownloadFor', { title: offer.title, host: offer.host })}
        busy={pending.has(`download:${offerKey(offer)}`)}
        onclick={() => act(`download:${offerKey(offer)}`, () => receive(offer.infoHash, offer.peerId))}
      >
        <Download size="1.4rem" strokeWidth={1.8} />
        {msg('transfers.lanDownload')}
      </Button>
    </div>
  </li>
{/snippet}

<Page>
  <PageHeader title={msg('transfers.lanTitle')} subtitle={msg('transfers.lanSubtitle')} />

  {#if loadFailed || retrying}
    <!-- On success focus that was on Retry goes to the first control of the sections, else the page heading. -->
    <div class="load-failed">
      <Notice tone="danger" bind:retrying retry={{ label: msg('common.retry'), run: reloadLan, within: () => content }}>
        {sourceErrorText($lanLoadError, msg('transfers.lanLoadFailed'))}
      </Notice>
    </div>
  {/if}

  {#if sharingOff}
    <div class="sharing-off">
      <Notice tone="info" announce={false} title={msg('transfers.lanSharingOffTitle')}>
        {msg('transfers.lanSharingOffText')}
        {#snippet action()}
          <Button size="sm" onclick={() => navigate('settings', { tab: 'general' })}>{msg('transfers.lanOpenSettings')}</Button>
        {/snippet}
      </Notice>
    </div>
  {/if}

  {#if !$lanLoaded}
    {#if loading}
      <EmptyState level={2} title={msg('transfers.lanLoading')} />
    {/if}
  {:else}
    <div class="content" bind:this={content}>
      <div class="stats">
        <div class="stat">
          <StatTile value={formatCount($lanStats.peersKnown)} label={msg('transfers.lanPeersLabel')}>
            {#snippet icon()}<MonitorSmartphone size="1.8rem" strokeWidth={1.8} />{/snippet}
          </StatTile>
        </div>
        <div class="stat">
          <StatTile value={formatCount($lanStats.offersKnown)} label={msg('transfers.lanOffersLabel')}>
            {#snippet icon()}<Wifi size="1.8rem" strokeWidth={1.8} />{/snippet}
          </StatTile>
        </div>
        <div class="stat">
          <StatTile value={formatCount($lanStats.sharesActive)} label={msg('transfers.lanSharesLabel')}>
            {#snippet icon()}<Share2 size="1.8rem" strokeWidth={1.8} />{/snippet}
          </StatTile>
        </div>
      </div>

      {#if !sharingOff}
        <section class="section" aria-labelledby={headings.nearby} data-lan-section="nearby">
          <SectionHeader id={headings.nearby} title={msg('transfers.lanNearbyHeading')} count={$offers.length} />
          {#if $offers.length === 0}
            <EmptyState level={3}
              title={msg('transfers.lanEmptyTitle')}
              description={msg('transfers.lanEmptyDescription')}
            >
              {#snippet icon()}
                <Wifi size="2rem" strokeWidth={1.8} />
              {/snippet}
            </EmptyState>
            {#if rejectedNote}
              <p class="note">{msg('transfers.lanRejectedPrefix', { summary: rejectedNote })}</p>
            {/if}
          {:else if groups.length === 1}
            <ul class="list">
              {#each $offers as offer (offerKey(offer))}
                {@render offerRow(offer, false)}
              {/each}
            </ul>
          {:else}
            {#each groups as group (group.peerId)}
              {@const id = `${uid}-host-${group.peerId}`}
              <section class="group" aria-labelledby={id}>
                <SectionHeader {id} level={3} title={msg('transfers.lanHostHeading', { host: group.host })} count={group.offers.length} />
                <ul class="list">
                  {#each group.offers as offer (offerKey(offer))}
                    {@render offerRow(offer, true)}
                  {/each}
                </ul>
              </section>
            {/each}
          {/if}
        </section>
      {/if}

      <section class="section" aria-labelledby={headings.sharing} data-lan-section="sharing">
        <SectionHeader id={headings.sharing} title={msg('transfers.lanSharingHeading')} count={$shares.length} />
        {#if $shares.length === 0 && preparing.length === 0}
          <EmptyState size="inline" title={msg('transfers.lanNoShares')} />
        {:else}
          <ul class="list">
            {#each preparing as progress (progress.gameId)}
              {@const title = sharedGameTitle(progress.gameId, $libraryGames)}
              {@const percent = progressPercent(progress.totalBytes > 0 ? progress.processedBytes / progress.totalBytes : 0)}
              <li class="row" data-lan-row={progress.gameId}>
                <div class="main">
                  <p class="title"><GameLink id={pageOf(progress.gameId)} name={title} /></p>
                  <ProgressBar
                    value={progress.totalBytes > 0 ? (progress.processedBytes / progress.totalBytes) * 100 : 0}
                    height={4}
                    label={title}
                    valueText={msg('transfers.lanPreparingPercent', { percent })}
                  />
                  <p class="sub">{msg('transfers.lanPreparingPercent', { percent })}</p>
                </div>
              </li>
            {/each}
            {#each $shares as share (share.gameId)}
              <li class="row share" data-lan-row={share.gameId}>
                <div class="main">
                  <p class="title"><GameLink id={pageOf(share.gameId)} name={share.title} /></p>
                </div>
                <span class="fact size">{bytesSize(share.sizeBytes)}</span>
                <span class="fact date">{relativeDate(share.builtAt)}</span>
                <div class="actions">
                  <Button
                    size="sm"
                    data-row-control="stop"
                    aria-label={msg('transfers.lanStopSharingFor', { title: share.title })}
                    busy={pending.has(`stop:${share.gameId}`)}
                    onclick={() => act(`stop:${share.gameId}`, () => unshare(share.gameId))}>{msg('transfers.lanStopSharing')}</Button
                  >
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="section" aria-labelledby={headings.transfers} data-lan-section="transfers">
        <SectionHeader id={headings.transfers} title={msg('transfers.lanTransfersHeading')} count={activeTransfers.length} />
        {#if activeTransfers.length === 0}
          <EmptyState size="inline" title={msg('transfers.lanNoTransfers')} />
        {:else}
          <ul class="list">
            {#each activeTransfers as transfer (transfer.id)}
              {@const amount = transferProgress(transfer)}
              <li class="row" data-lan-row={transfer.id}>
                <div class="main">
                  <p class="title">{transfer.title}</p>
                  <ProgressBar
                    value={amount?.value ?? 0}
                    indeterminate={amount === null}
                    height={4}
                    label={transfer.title}
                    valueText={amount?.text}
                  />
                  <p class="sub">{transferLabel(transfer)}</p>
                </div>
                <div class="actions">
                  <IconButton
                    size="sm"
                    data-row-control="cancel"
                    label={msg('transfers.lanCancelFor', { title: transfer.title })}
                    busy={pending.has(`cancel:${transfer.id}`)}
                    onclick={() => askCancel(transfer)}
                  >
                    <X size="1.5rem" strokeWidth={1.8} />
                  </IconButton>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  {/if}

  {#if asked}
    {@const { transfer, place } = asked}
    <ConfirmModal
      prompt={cancelPrompt(transfer)}
      onconfirm={() => cancelTransfer(transfer.id)}
      onclose={() => (asking = null)}
      errorText={(err) => sourceErrorText(err)}
      returnFocus={() => returnTo(place)}
    />
  {/if}
</Page>

<style>
  .load-failed,
  .sharing-off {
    margin-bottom: var(--space-5);
  }

  .stats {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: flex-start;
    row-gap: var(--space-4);
    margin-bottom: var(--space-8);
    padding: var(--space-4) var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
  }

  .stat {
    min-width: 14rem;
    padding: 0 var(--space-6);
    border-left: 1px solid var(--border);
  }

  .stat:first-child {
    padding-left: 0;
    border-left: 0;
  }

  .section {
    max-width: 140rem;
    margin-bottom: var(--space-10);
  }

  .section:last-child {
    margin-bottom: 0;
  }

  .group + .group {
    margin-top: var(--space-5);
  }

  .note {
    margin: var(--space-2) 0 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .list {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
    overflow: hidden;
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-height: 6.4rem;
    padding: var(--space-2) var(--space-5);
    transition: background var(--dur) var(--ease);
  }

  .row:not(:first-child) {
    border-top: 1px solid var(--border);
  }

  .row:hover {
    background: var(--hover);
  }

  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .main p {
    margin: 0;
  }

  /* Shown whole, wrapping in the row: an ellipsis would hide words no keyboard could reach. */
  .title {
    font-size: var(--font-md);
    font-weight: 500;
    overflow-wrap: anywhere;
  }

  .sub {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .fact {
    font-size: var(--font-sm);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    text-align: right;
  }

  .size {
    min-width: 7rem;
  }

  /* The version tells two offers of the same game apart; the date says when a share was prepared. */
  .date,
  .version {
    min-width: 9rem;
  }

  .actions {
    display: flex;
    margin-left: var(--space-2);
  }

  /* A narrow window wraps a share's date under its title: nothing else says it. */
  @media (max-width: 1300px) {
    .share {
      flex-wrap: wrap;
      align-content: center;
      row-gap: 0;
    }

    .share .date {
      order: 1;
      flex-basis: 100%;
      min-width: 0;
      text-align: left;
    }
  }
</style>
