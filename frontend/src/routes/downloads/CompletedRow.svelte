<script lang="ts">
  import { Ellipsis } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import Reading from '../../lib/components/Reading.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import type { MenuItem } from '../../lib/components/menu';
  import { locale, t } from '../../lib/i18n';
  import { canRetryInstall, installErrorText, needsInteractiveInstall } from '../../lib/install/installErrors';
  import { installIndeterminate } from '../../lib/install/progress';
  import { canPlay } from '../../lib/library/launch';
  import { libraryErrorText } from '../../lib/library/errors';
  import { gamePageId, type Download } from '../../lib/services/downloads';
  import { retryInstall, type Installation } from '../../lib/services/install';
  import { launchesInFlight, playGame } from '../../lib/services/library';
  import { openFolder } from '../../lib/services/settings';
  import { installStatusLabels } from '../../lib/stores/install';
  import { libraryGamesById, runningGames } from '../../lib/stores/library';
  import { gameArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { sources } from '../../lib/stores/sources';
  import { toast } from '../../lib/stores/toasts';
  import { makeFormat, progressPercent } from '../../lib/utils/format';
  import { completedWhen, installedGame, type CompletedKind } from './completedRow';
  import { coverOf, sourceTag, typeTag } from './downloadRow';

  /**
   * One completed download on the Downloads screen: cover and title (the game's page), the type, the
   * size, when it finished and its source, what became of it (`kind`), one lead button and a "more"
   * menu. A plain list item: every control is a sibling named after the download.
   *
   * The lead is one button whose words and action follow the kind (Details, Continue, Retry, Install,
   * Play, the running game's page), so a press that changes the kind leaves focus on it. A row that
   * leaves its section under the player's press (Retry moves it to "Installing", Remove drops it) says
   * so through `onleave` before the request and with the outcome after it; the section hands focus on.
   */
  let {
    download,
    installation,
    kind,
    onopen,
    oninstall,
    onremove,
    onleave,
  }: {
    download: Download;
    installation: Installation | undefined;
    kind: CompletedKind;
    /** Opens the download's details. */
    onopen: (id: string) => void;
    /** Opens the install dialog of the download. */
    oninstall: (id: string) => void;
    /** Asks Go to remove the download from the list. */
    onremove: (download: Download) => void;
    /** Called at a press that may move the row away; the answer is called with whether Go took it. */
    onleave: (id: string) => (ok: boolean) => void;
  } = $props();

  const id = $derived(download.id);
  const name = $derived(download.name);
  const gameId = $derived(gamePageId(download));
  const format = $derived(makeFormat($locale, $t));
  const when = $derived(completedWhen(download.completedAt, new Date(), format, $t));
  const installedWhen = $derived(completedWhen(installation?.completedAt, new Date(), format, $t));
  const type = $derived(typeTag(download));
  const source = $derived(sourceTag(download, $sources));

  const game = $derived(kind === 'installed' ? installedGame(installation, $libraryGamesById) : undefined);
  const running = $derived(Boolean(game && $runningGames.has(game.id)));
  const playable = $derived(Boolean(game && canPlay(game)));

  let pressed = $state<'retry' | 'play' | null>(null);
  const launching = $derived(pressed === 'play' || Boolean(game && $launchesInFlight.has(game.id)));

  // A failure Go can retry on its own is retried from here; one that needs the player (an installer to
  // run by hand, a game already in the library) goes to the install dialog, which offers what fits.
  const retriesHere = $derived(
    kind === 'failed' && Boolean(installation) && canRetryInstall(installation?.error) && !needsInteractiveInstall(installation?.error),
  );

  interface Lead {
    text: string;
    name: string;
    variant: 'primary' | 'play' | 'secondary' | 'ghost' | 'danger';
    busy: boolean;
    run: () => void;
  }

  const lead = $derived.by((): Lead | null => {
    const named = (text: string, label: string, variant: Lead['variant'], run: () => void, busy = false): Lead => ({
      text,
      name: label,
      variant,
      busy,
      run,
    });
    switch (kind) {
      case 'installing':
        return named($t('downloads.done.installDetails'), $t('downloads.done.installDetailsNamed', { name }), 'ghost', () => oninstall(id));
      case 'waiting':
        return named($t('downloads.done.continue'), $t('downloads.done.continueNamed', { name }), 'primary', () => oninstall(id));
      case 'failed':
        return retriesHere
          ? named($t('downloads.done.retry'), $t('downloads.done.retryNamed', { name }), 'danger', retry, pressed === 'retry')
          : named($t('downloads.done.installDetails'), $t('downloads.done.installDetailsNamed', { name }), 'danger', () => oninstall(id));
      case 'stopped':
        return named($t('downloads.done.installAgain'), $t('downloads.done.installAgainNamed', { name }), 'secondary', () => oninstall(id));
      case 'ready':
        return named($t('downloads.done.install'), $t('downloads.done.installNamed', { name }), 'primary', () => oninstall(id));
      case 'installed':
        if (!game) return null;
        if (running) return named($t('downloads.done.running'), $t('downloads.done.runningNamed', { name }), 'ghost', openRunning);
        if (!playable) return null;
        return named($t('downloads.done.play'), $t('downloads.done.playNamed', { name }), 'play', play, launching);
      default:
        return null;
    }
  });

  async function retry() {
    const item = installation;
    if (!item || pressed) return;
    pressed = 'retry';
    const answer = onleave(id);
    try {
      await retryInstall(item.id);
      answer(true);
    } catch (err) {
      answer(false);
      toast(installErrorText(err), 'danger');
    } finally {
      pressed = null;
    }
  }

  async function play() {
    const target = game;
    if (!target || pressed || launching) return;
    pressed = 'play';
    try {
      await playGame(target.id);
    } catch (err) {
      toast(libraryErrorText(err, $t('games.errorPlayFailed')), 'danger');
    } finally {
      pressed = null;
    }
  }

  function openRunning() {
    if (game) navigate('game', { id: game.id });
  }

  async function openDestination() {
    try {
      await openFolder(download.destination);
    } catch {
      toast($t('transfers.downloadsFolderUnavailable'), 'danger');
    }
  }

  const menuItems = $derived.by((): MenuItem[] => {
    const items: MenuItem[] = [{ id: 'details', label: $t('downloads.panel.details') }];
    if (kind === 'failed' && retriesHere) items.push({ id: 'install', label: $t('downloads.done.installView') });
    items.push({ id: 'folder', label: $t('downloads.panel.openFolder') });
    items.push({
      id: 'remove',
      label: download.seeding ? $t('downloads.done.removeSeeding') : $t('downloads.done.remove'),
      separator: true,
    });
    return items;
  });

  function onmenu(item: string) {
    if (item === 'details') onopen(id);
    else if (item === 'install') oninstall(id);
    else if (item === 'folder') void openDestination();
    else if (item === 'remove') onremove(download);
  }

  const stageWords = $derived.by(() => {
    if (!installation || kind !== 'installing') return '';
    const stage = installStatusLabels(installation.status);
    if (installIndeterminate(installation)) return stage;
    return $t('downloads.done.installing', { stage, percent: progressPercent(installation.progress) });
  });
</script>

<li class="row" data-download-row={id} data-kind={kind}>
  <div class="cover">
    <GameLink id={gameId} name={name} repeat>
      <Artwork src={coverOf(download, $gameArt)} alt="" label={name} seed={download.origin.gameId || id} ratio="3 / 4" radius="var(--radius-sm)" />
    </GameLink>
  </div>

  <div class="main">
    <h3 class="title wraps"><GameLink id={gameId} {name} /></h3>

    <p class="facts">
      {#if type}<StatusBadge kind="neutral" label={type} dot={false} />{/if}
      {#if download.origin.platform}<ConsoleTag platform={download.origin.platform} />{/if}
      {#if download.total > 0}<span class="fact"><Reading text={format.bytesSize(download.total)} /></span>{/if}
      {#if when}
        <!-- The exact time goes to assistive tech as text and to the pointer as a hint; the time element
             takes focus only from the pointer, so the row adds no Tab stop for it. -->
        <Tooltip text={when.exact} describe={false}>
          <time class="fact" datetime={download.completedAt} tabindex="-1"><span aria-hidden="true">{when.words}</span><span class="sr-only">{when.exact}</span></time>
        </Tooltip>
      {/if}
      {#if source}<span class="fact">{source}</span>{/if}
    </p>

    {#if kind === 'installing' && installation}
      <p class="state"><Reading text={stageWords} /></p>
      <div class="bar">
        <ProgressBar
          value={installation.progress * 100}
          indeterminate={installIndeterminate(installation)}
          height={4}
          label={$t('downloads.done.installOf', { name })}
          active
        />
      </div>
    {:else if kind === 'waiting'}
      <p class="state">{$t('downloads.done.waiting')}</p>
    {:else if kind === 'failed' && installation}
      <p class="state">{$t('downloads.done.failed', { error: installErrorText(installation.error) })}</p>
    {:else if kind === 'stopped' && installation}
      <p class="state">{installation.status === 'cancelled' ? $t('downloads.done.cancelled') : $t('downloads.done.interrupted')}</p>
    {:else if kind === 'installed' && installation}
      <p class="state">
        {#if installation.warning}
          <StatusBadge kind="warning" label={$t('downloads.done.installed')} hint={installation.warning} plain />
        {:else}
          <StatusBadge kind="success" label={$t('downloads.done.installed')} plain />
        {/if}
        {#if installedWhen}<span>{$t('downloads.done.installedAt', { when: installedWhen.words })}</span>{/if}
      </p>
    {/if}

    {#if download.seeding}
      <p class="state seeding">
        <Reading text={download.uploadSpeed > 0 ? $t('downloads.done.seedingAt', { speed: format.speedBytes(download.uploadSpeed) }) : $t('downloads.done.seeding')} />
      </p>
    {/if}
  </div>

  <div class="actions">
    {#if lead}
      <Button variant={lead.variant} size="sm" busy={lead.busy} aria-label={lead.name} data-row-control="lead" onclick={lead.run}>
        {lead.text}
      </Button>
    {/if}
    <DropdownMenu items={menuItems} onselect={onmenu}>
      {#snippet trigger({ props })}
        <IconButton label={$t('downloads.panel.more', { name })} size="sm" {...props} data-row-control="more">
          <Ellipsis size="1.6rem" strokeWidth={1.8} />
        </IconButton>
      {/snippet}
    </DropdownMenu>
  </div>
</li>

<style>
  .row {
    display: grid;
    grid-template-columns: 4.8rem minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-2);
  }

  .row:not(:first-child) {
    border-top: 1px solid var(--border);
  }

  .cover {
    overflow: hidden;
    border-radius: var(--radius-sm);
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .title {
    margin: 0;
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

  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    margin: 0;
    color: var(--text-2);
    font-size: var(--font-xs);
    font-variant-numeric: tabular-nums;
  }

  .state {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    color: var(--text-2);
    font-size: var(--font-sm);
    font-variant-numeric: tabular-nums;
  }

  .bar {
    max-width: 32rem;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  @media (max-width: 560px) {
    .row {
      grid-template-columns: 3.6rem minmax(0, 1fr);
    }

    .actions {
      grid-column: 2;
    }
  }
</style>
