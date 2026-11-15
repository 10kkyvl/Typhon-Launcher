<script lang="ts">
  import { onDestroy } from 'svelte';
  import { Ellipsis, Pause, Play } from '@lucide/svelte';
  import { t } from '../i18n';
  import { installErrorText } from '../install/installErrors';
  import { cancelDownloadPrompt } from '../confirm/prompts';
  import { cancelAsks } from '../downloads/cancel';
  import { heldBack } from '../downloads/schedule';
  import { emptySpeedSeries, type SpeedSeries } from '../downloads/speedHistory';
  import { gamePageId, type Download } from '../services/downloads';
  import { openFolder } from '../services/settings';
  import { requestDownloadAction, speedHistory, statusLabels } from '../stores/downloads';
  import { downloadOffers, waitingForSchedule } from '../stores/downloadOffers';
  import { gameScope } from '../stores/gameTone';
  import { gameArt, requestArt } from '../stores/metadata';
  import { playThrottle } from '../stores/playThrottle';
  import { settings } from '../stores/settings';
  import { sources } from '../stores/sources';
  import { toast } from '../stores/toasts';
  import { bytesSize, etaLabel, progressPercent, rateLimitLabel, speedBytes } from '../utils/format';
  import Artwork from './Artwork.svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import ConsoleTag from './ConsoleTag.svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import GameLink from './GameLink.svelte';
  import IconButton from './IconButton.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import Reading from './Reading.svelte';
  import SpeedGraph from './SpeedGraph.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import type { MenuItem } from './menu';
  import { leadAction, panelControl, speedLimit, type Leave, type LeaveTicket } from './downloadPanel';

  /**
   * The panel of a download under way on the Downloads screen, one per download of the store's `active`
   * list: cover, tags, title, progress, the speed, time left and peers, one lead button and a "more"
   * menu, and the speed graph of the last 40 seconds. A download that moves data or is about to
   * (downloading, metadata, verifying) is the full panel; a paused one the compact form, without the
   * graph and the speed, which nothing moves. The form follows the status (`data-form`); the controls are
   * the same nodes in both, so a download changing form keeps focus where it was.
   *
   * A plain container: every control is a sibling named after the download. The lead button is one
   * button whose action follows what Go offers (Download now, Pause, Resume); while its call runs it is
   * busy and keeps focus. When the panel goes from under the focus its own press left there (a cancel, a
   * resume that queues it), focus goes on by the screen's rule, `leave`, once the panel has gone, whichever
   * of Go's answer and its event comes first. Cancel is Go's Cancel and asks first when data exists.
   */
  let {
    download,
    onopen,
    onannounce,
    leave,
  }: {
    download: Download;
    /** Opens the download's details. */
    onopen?: (download: Download) => void;
    /** Says what the player did, in the page's one polite live region. */
    onannounce?: (text: string) => void;
    /** The screen's rule for focus when a row leaves (routes/downloads/rowFocus.ts::pressed). */
    leave?: Leave;
  } = $props();

  const uid = $props.id();
  const titleId = `${uid}-title`;

  const id = $derived(download.id);
  const gameId = $derived(gamePageId(download));
  const artId = $derived(download.origin.gameId ?? '');
  const cover = $derived((artId && $gameArt[artId]?.cover) || '');
  const sourceName = $derived($sources.find((s) => s.id === download.origin.sourceId)?.name ?? '');
  const typeTag = $derived.by(() => {
    if (download.origin.platform) return $t('state.installTypeImage');
    if (download.origin.purpose === 'update') return $t('ui.update');
    if (download.origin.purpose === 'repair') return $t('ui.repair');
    if (download.origin.gameId || download.origin.releaseId) return $t('ui.game');
    return '';
  });

  const offers = $derived($downloadOffers.get(id) ?? []);
  const lead = $derived(leadAction(offers));
  const waiting = $derived($waitingForSchedule.has(id));
  const compact = $derived(download.status === 'paused');
  const downloading = $derived(download.status === 'downloading');
  const metadata = $derived(download.status === 'metadata');
  const limit = $derived(speedLimit($settings, $playThrottle));

  const percent = $derived(progressPercent(download.progress));
  const amountParams = $derived({ done: bytesSize(download.downloaded), total: bytesSize(download.total), percent });
  const amount = $derived($t('downloads.panel.amount', amountParams));
  const valueText = $derived(
    download.status === 'paused'
      ? $t('downloads.panel.amountPaused', amountParams)
      : download.status === 'verifying'
        ? $t('downloads.panel.amountChecking', amountParams)
        : amount,
  );

  const leadLabel = $derived(
    lead === 'downloadNow'
      ? { text: $t('downloads.panel.downloadNow'), name: $t('downloads.panel.downloadNowNamed', { name: download.name }) }
      : lead === 'pause'
        ? { text: $t('downloads.panel.pause'), name: $t('downloads.panel.pauseNamed', { name: download.name }) }
        : { text: $t('downloads.panel.resume'), name: $t('downloads.panel.resumeNamed', { name: download.name }) },
  );

  // "Download now" is the lead whenever Go offers it, so the menu does not repeat it; Pause, which Go
  // still takes then, moves into the menu.
  const menuItems = $derived.by((): MenuItem[] => {
    const items: MenuItem[] = [{ id: 'details', label: $t('downloads.panel.details') }];
    if (lead === 'downloadNow' && offers.includes('pause')) items.push({ id: 'pause', label: $t('downloads.panel.pause') });
    if (offers.includes('forceStart') && !heldBack(download, $playThrottle)) {
      items.push({ id: 'forceStart', label: $t('downloads.panel.startNow') });
    }
    items.push({ id: 'folder', label: $t('downloads.panel.openFolder') });
    if (offers.includes('cancel')) items.push({ id: 'cancel', label: $t('downloads.panel.cancel'), danger: true, separator: true });
    return items;
  });

  $effect(() => {
    if (artId) requestArt([artId]);
  });

  // One reader of the speed series per panel while it draws the graph; none in the compact form.
  let series = $state<SpeedSeries>(emptySpeedSeries());
  $effect(() => {
    if (compact) return;
    return speedHistory(id).subscribe((next) => {
      series = next;
    });
  });

  let leadBusy = $state(false);

  // A press Go took whose panel may still leave: its hand-off waits for the panel to go. A lead press
  // whose effect shows on a panel that stays (Pause turned Resume) needs none, so its wait ends there.
  let held: { ticket: LeaveTicket; lead?: string | null } | null = null;
  let gone = false;

  onDestroy(() => {
    gone = true;
    const press = held;
    held = null;
    if (press) void press.ticket.handOn();
    // A cancel still asked about ends with the panel: the dialog goes with it and hands focus on itself.
    if (asking && !asking.done) asking.ticket?.drop();
  });

  $effect(() => {
    const now = lead;
    if (held?.lead !== undefined && held.lead !== now) {
      held.ticket.drop();
      held = null;
    }
  });

  function untilGone(ticket: LeaveTicket | undefined, pressedLead?: string | null) {
    if (!ticket) return;
    if (gone) void ticket.handOn();
    else held = { ticket, lead: pressedLead };
  }

  async function pressLead() {
    const action = lead;
    if (!action || leadBusy) return;
    const own = id;
    const ticket = leave?.(own, { follow: 'first', prefer: panelControl('lead') });
    leadBusy = true;
    const outcome = await requestDownloadAction(own, action);
    leadBusy = false;
    if (!outcome.ok) {
      ticket?.drop();
      return;
    }
    // Already shown on a panel that stays: nothing left to hand on.
    if (!gone && lead !== action) ticket?.drop();
    else untilGone(ticket, action);
  }

  async function openDestination() {
    try {
      await openFolder(download.destination);
    } catch {
      toast($t('transfers.downloadsFolderUnavailable'), 'danger');
    }
  }

  let confirmOpen = $state(false);
  let asking: { ticket: LeaveTicket | undefined; done: boolean } | null = null;

  function cancelled(name: string) {
    onannounce?.($t('downloads.cancel.done', { name }));
  }

  async function cancelNow(ticket: LeaveTicket | undefined) {
    const own = id;
    const name = download.name;
    const outcome = await requestDownloadAction(own, 'cancel');
    if (!outcome.ok) {
      ticket?.drop();
      return;
    }
    cancelled(name);
    untilGone(ticket);
  }

  function askCancel() {
    const ticket = leave?.(id, { prefer: panelControl('more') });
    if (!cancelAsks(download)) {
      void cancelNow(ticket);
      return;
    }
    asking = { ticket, done: false };
    confirmOpen = true;
  }

  // The dialog shows a refusal itself, by its code, and stays open for another try.
  async function confirmCancel() {
    const current = asking;
    const own = id;
    const name = download.name;
    const outcome = await requestDownloadAction(own, 'cancel', { toast: false });
    if (!outcome.ok) throw outcome.error;
    cancelled(name);
    if (!current) return;
    current.done = true;
    untilGone(current.ticket);
  }

  function closeCancel() {
    confirmOpen = false;
    if (asking && !asking.done) asking.ticket?.drop();
    asking = null;
  }

  const cancelPrompt = $derived(cancelDownloadPrompt(download));

  function onmenu(item: string) {
    if (item === 'details') onopen?.(download);
    else if (item === 'pause') void requestDownloadAction(id, 'pause');
    else if (item === 'forceStart') void requestDownloadAction(id, 'forceStart');
    else if (item === 'folder') void openDestination();
    else if (item === 'cancel') askCancel();
  }
</script>

<section
  class="item"
  class:compact
  aria-labelledby={titleId}
  data-form={compact ? 'compact' : 'full'}
  data-state={download.status}
  data-download-row={id}
  {@attach gameScope(gameId)}
>
  <div class="cover">
    <GameLink id={gameId} name={download.name} repeat>
      <Artwork src={cover} alt="" label={download.name} seed={artId || id} ratio="3 / 4" radius="var(--radius-sm)" />
    </GameLink>
  </div>

  <div class="main">
    <div class="tags">
      {#if !downloading}<StatusBadge kind={compact ? 'warning' : 'accent'} label={statusLabels(download.status)} />{/if}
      {#if typeTag}<StatusBadge kind="neutral" label={typeTag} dot={false} />{/if}
      {#if download.origin.platform}<ConsoleTag platform={download.origin.platform} />{/if}
      {#if download.total > 0}<span class="tag"><Reading text={bytesSize(download.total)} /></span>{/if}
      {#if sourceName}<span class="tag">{sourceName}</span>{/if}
      {#if download.ignoreSchedule}<StatusBadge kind="neutral" label={$t('transfers.outsideSchedule')} dot={false} />{/if}
    </div>

    <h3 class="title" id={titleId}><GameLink id={gameId} name={download.name} /></h3>

    <ProgressBar
      value={download.progress * 100}
      indeterminate={metadata}
      height={6}
      tone="game"
      label={$t('downloads.panel.progressOf', { name: download.name })}
      {valueText}
      active={downloading && download.downloadSpeed > 0 && !waiting}
      paused={compact}
    />
    {#if !metadata}
      <p class="amount" aria-hidden="true" data-testid="download-amount"><Reading text={amount} /></p>
    {/if}

    {#if waiting}
      <p class="note">{$t('downloads.panel.scheduleNote')}</p>
    {:else if downloading}
      <p class="stats">
        <span class="speed" data-testid="download-speed"><span class="sr-only">{$t('downloads.panel.speed')}</span><Reading text={speedBytes(download.downloadSpeed)} /></span>
        {#if limit > 0}<span class="sub"><Reading text={$t('downloads.panel.limit', { limit: rateLimitLabel(limit) })} /></span>{/if}
        <span class="rest">
          {#if download.stalled}
            <span class="stalled">{$t('ui.awaitingSources')}</span>
          {:else if download.etaSeconds >= 0}
            <Reading text={$t('downloads.panel.eta', { eta: etaLabel(download.etaSeconds) })} />
          {/if}
          <span class="dim"><Reading text={$t('downloads.panel.peers', { peers: download.peers, seeders: download.seeders })} /></span>
          <span class="dim"><Reading text={$t('downloads.panel.upload', { speed: speedBytes(download.uploadSpeed) })} /></span>
        </span>
      </p>
    {/if}

    <div class="actions">
      {#if lead}
        <Button
          variant={lead === 'pause' ? 'secondary' : 'primary'}
          size="sm"
          busy={leadBusy}
          aria-label={leadLabel.name}
          data-panel-control="lead"
          onclick={pressLead}
        >
          {#if lead === 'pause'}<Pause size="1.5rem" strokeWidth={1.8} aria-hidden="true" />{:else}<Play size="1.5rem" strokeWidth={1.8} aria-hidden="true" />{/if}
          {leadLabel.text}
        </Button>
      {/if}
      <DropdownMenu items={menuItems} align="left" onselect={onmenu}>
        {#snippet trigger({ props })}
          <IconButton label={$t('downloads.panel.more', { name: download.name })} size="sm" {...props} data-panel-control="more">
            <Ellipsis size="1.6rem" strokeWidth={1.8} />
          </IconButton>
        {/snippet}
      </DropdownMenu>
    </div>
  </div>

  {#if !compact}
    <div class="graph">
      <SpeedGraph {series} {limit} testid="download-graph" />
    </div>
  {/if}
</section>

{#if confirmOpen}
  <ConfirmModal
    prompt={cancelPrompt}
    onconfirm={confirmCancel}
    onclose={closeCancel}
    errorText={(err) => installErrorText(err)}
    returnFocus={() => asking?.ticket?.target()}
  />
{/if}

<style>
  .item {
    display: grid;
    grid-template-columns: 12rem minmax(0, 1fr) minmax(20rem, 28rem);
    gap: var(--space-5);
    align-items: start;
    padding: var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .item.compact {
    grid-template-columns: 6.4rem minmax(0, 1fr);
    padding: var(--space-3) var(--space-5);
  }

  .cover {
    overflow: hidden;
    border-radius: var(--radius-sm);
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    color: var(--text-2);
    font-size: var(--font-xs);
  }

  .tags:empty {
    display: none;
  }

  .tag {
    font-variant-numeric: tabular-nums;
  }

  .title {
    margin: 0;
    font-size: var(--font-lg);
    font-weight: 650;
    letter-spacing: var(--tracking-heading);
    overflow-wrap: anywhere;
  }

  .compact .title {
    font-size: var(--font-md);
  }

  .amount,
  .note {
    margin: 0;
    color: var(--text-2);
    font-size: var(--font-sm);
  }

  .stats {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-1) var(--space-4);
    margin: var(--space-1) 0 0;
    font-variant-numeric: tabular-nums;
  }

  .speed {
    font-size: var(--font-title);
    font-weight: 650;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
  }

  .sub {
    color: var(--text-2);
    font-size: var(--font-xs);
  }

  .rest {
    display: inline-flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    color: var(--text-2);
    font-size: var(--font-sm);
  }

  .dim {
    color: var(--text-3);
  }

  .stalled {
    color: var(--warning);
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-1);
  }

  .graph {
    min-width: 0;
  }

  @media (max-width: 1100px) {
    .item {
      grid-template-columns: 9.6rem minmax(0, 1fr);
    }

    .graph {
      grid-column: 1 / -1;
    }
  }
</style>
