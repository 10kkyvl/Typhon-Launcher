<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { locale } from '../../lib/i18n/locale';
  import { tick, untrack } from 'svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { tabPanel } from '../../lib/components/tabs';
  import type UpdateCard from '../../lib/components/UpdateCard.svelte';
  import GameActions from './GameActions.svelte';
  import GameDialogs from './GameDialogs.svelte';
  import GamePageHero from './GamePageHero.svelte';
  import GameMetaNotice from './GameMetaNotice.svelte';
  import GameScreenshots from './GameScreenshots.svelte';
  import GameSections from './GameSections.svelte';
  import { createGamePageData } from './gamePageData.svelte';
  import { discardDownloadPrompt, removeDownloadPrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import { firstWebUrl, releaseDownloadOrigin, sharedPlatform, splitReleases, type ReleaseOwner } from '../../lib/game/releases';
  import { catalogLoadKey, pageTitle, releaseLoadKey, sameGameIds } from '../../lib/game/pageLoad';
  import { busyState, cancelFreesDisk, clean, galleryShots, hubAction, isGameMissing, notOnPcBadge, onPC, pickHero } from '../../lib/game/view';
  import { activeDownload, activeInstall, gameDownloads, gameInstalls, ownsDownload as ownsGameDownload, terminalDownload as newestTerminalDownload } from '../../lib/game/transfers';
  import { cancelDownload, deleteDownloadData, removeDownload, resumeDownload } from '../../lib/services/downloads';
  import type { Download as DownloadItem, DownloadOrigin } from '../../lib/services/downloads';
  import { missingImages, refreshMissingImages } from '../../lib/stores/missingImages';
  import { prepareReleaseDownload, type ReleaseGroup } from '../../lib/services/sources';
  import { getVerifyState, type VerifyState } from '../../lib/services/updates';
  import { updateErrorText } from '../../lib/updates/updateErrors';
  import { downloads, statusLabels } from '../../lib/stores/downloads';
  import { gameSections, type GameSection } from '../../lib/theme/layout';
  import { activeTheme } from '../../lib/stores/theme';
  import { installStatusLabels, installations } from '../../lib/stores/install';
  import { installIndeterminate } from '../../lib/install/progress';
  import { libraryGames, runningGames } from '../../lib/stores/library';
  import { emulationPlatform, isEmulated, pageGame } from '../../lib/library/emulation';
  import { externalLabel, isExternal } from '../../lib/library/external';
  import { canPlay } from '../../lib/library/launch';
  import { metadataAvailable } from '../../lib/stores/metadata';
  import { claimAmbient } from '../../lib/stores/ambient';
  import { pageAmbient } from '../../lib/backdrop/source';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { openSaveBackups } from '../../lib/stores/savebackup';
  import { activeMove } from '../../lib/stores/relocate';
  import { sources } from '../../lib/stores/sources';
  import { stepLabels, updatesByGame, verifications } from '../../lib/stores/updates';
  import { msg } from '../../lib/i18n';
  import { installErrorText } from '../../lib/install/installErrors';
  import { sourceErrorText } from '../../lib/sources/sourceErrors';

  let { id }: { id: string } = $props();

  const uid = $props.id();

  const pageOwn = $derived(
    $libraryGames.find((g) => g.id === id) ?? $libraryGames.find((g) => g.canonicalGameId === id || catalogGame?.aliasIds?.includes(g.canonicalGameId ?? '')),
  );
  const found = $derived.by(() => pageGame(pageOwn, $libraryGames, [id, ...(catalogGame?.aliasIds ?? []), ...(data.relatedFailed ? [] : sameGameIds(data.related))]));
  const localGame = $derived(found.game);
  const viaCatalog = $derived(found.viaCatalog);
  const wasInstalled = $derived(Boolean(localGame?.installedAt && Date.parse(localGame.installedAt) > 0));
  const installed = $derived(Boolean(localGame) && !localGame?.uninstalled);
  const running = $derived(localGame ? $runningGames.has(localGame.id) : false);

  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('library');
  let statusOpen = $state(false);
  let tagsOpen = $state(false);

  const external = $derived(isExternal(localGame));
  const externalName = $derived(externalLabel(localGame));
  const emulated = $derived(isEmulated(localGame));
  const emulatedPlatform = $derived(emulationPlatform(localGame));
  const managed = $derived(!external && !emulated);
  const update = $derived(localGame && managed ? $updatesByGame.get(localGame.id) : undefined);
  const verifyState = $derived(localGame ? $verifications[localGame.id] : undefined);
  const updateAvailable = $derived(Boolean(update?.availability.available));
  const updateIsRelease = $derived(update?.availability.kind !== 'update');

  let updateBadgePopped = false;

  $effect(() => {
    if (!updateAvailable) updateBadgePopped = false;
  });

  function popOnce(node: HTMLElement) {
    if (updateBadgePopped) return;
    updateBadgePopped = true;
    node.classList.add('fresh');
  }

  let updateCard = $state<UpdateCard | undefined>(undefined);

  async function loadVerifyState(gameId: string) {
    let state: VerifyState | null;
    try {
      state = await getVerifyState(gameId);
    } catch (err) {
      toast(updateErrorText(err, msg('verify.stateReadFailed')), 'danger');
      return;
    }
    if (!state) return;
    verifications.update((map) => (map[gameId] ? map : { ...map, [gameId]: state }));
  }

  $effect(() => {
    const gameId = localGame?.id;
    if (!gameId || emulated) return;
    untrack(() => {
      loadVerifyState(gameId);
    });
  });

  let titleHeading = $state<HTMLElement>();
  let otherVersionsEl = $state<HTMLElement | null>(null);

  // Created after the verify-state effect and before the effects below, so that the service calls of
  // the page keep their order.
  const data = createGamePageData({
    get id() { return id; },
    get catalogKey() { return catalogKey; },
    get canonicalId() { return canonicalId; },
    get locale() { return $locale; },
    get sources() { return $sources; },
    get metadataAvailable() { return $metadataAvailable; },
    get releaseKey() { return releaseKey; },
    get releaseTitle() { return releaseTitle; },
    get title() { return title; },
    get coverSrc() { return coverSrc; },
    get external() { return external; },
    get sideloaded() { return sideloaded; },
    get localGame() { return localGame; },
    get emulatedPlatform() { return emulatedPlatform; },
    get titleHeading() { return titleHeading; },
  });
  // Read here under their own names; they are written only through `data`.
  const catalogGame = $derived(data.catalogGame);
  const metaView = $derived(data.metaView);

  const catalogKey = $derived(catalogLoadKey(id, $libraryGames));

  const canonicalId = $derived(localGame && !viaCatalog ? localGame.canonicalGameId : catalogGame?.id || id);
  const releaseTitle = $derived(pageTitle(localGame, viaCatalog, catalogGame?.title));
  const sideloaded = $derived(emulated && !viaCatalog);
  const releaseKey = $derived(
    releaseLoadKey({ id, canonicalId, title: releaseTitle, sideloaded, external: external }),
  );

  const anyOwnDownload = $derived($downloads.find((item) => ownsDownload(item)));

  let matchOpen = $state(false);
  let matchMode = $state<'find' | 'change'>('find');
  let summaryExpanded = $state(false);
  let lightboxOpen = $state(false);
  let lightboxIndex = $state(0);
  let pickerOpen = $state(false);
  let heroFailed = $state(false);

  const info = $derived(metaView?.game ?? catalogGame ?? null);
  const title = $derived(
    clean(info?.title) || clean(catalogGame?.title) || clean(localGame?.title) || clean(anyOwnDownload?.name),
  );
  const screenshots = $derived(metaView?.screenshots ?? []);
  const heroSrc = $derived(pickHero(metaView));
  const shots = $derived(galleryShots(screenshots, heroSrc));
  const coverSrc = $derived(clean(metaView?.cover) || clean(catalogGame?.coverUrl) || clean(localGame?.cover));
  const showHero = $derived(Boolean(heroSrc) && !heroFailed);

  $effect(() => {
    heroSrc;
    untrack(() => {
      heroFailed = false;
      lightboxIndex = 0;
    });
  });

  const shotCols = $derived(
    shots.length >= 5 ? 4 : shots.length === 4 ? 2 : Math.max(1, shots.length),
  );

  let activeTab = $state('overview');

  const ownShots = $derived(Boolean(localGame) && installed);
  const hasShotsTab = $derived(shots.length > 0 || ownShots);

  const tabs = $derived([
    { id: 'overview', label: msg('games.detailTabOverview') },
    ...(hasShotsTab ? [{ id: 'screenshots', label: msg('games.detailTabScreenshots') }] : []),
  ]);

  $effect(() => {
    if (activeTab === 'screenshots' && !hasShotsTab) activeTab = 'overview';
  });

  const owner = $derived({ gameIds: [canonicalId], libraryId: localGame?.id });
  function ownsDownload(item: DownloadItem) {
    return ownsGameDownload(item, owner);
  }

  const ownDownloads = $derived(gameDownloads($downloads, owner));
  const ownDownload = $derived(activeDownload(ownDownloads));
  const terminalDownload = $derived(installed ? undefined : newestTerminalDownload(ownDownloads));
  const ownInstall = $derived(activeInstall(gameInstalls($installations, ownDownloads, localGame?.id)));

  const ownMove = $derived(localGame && $activeMove?.gameId === localGame.id ? $activeMove : null);

  // An installed game keeps its main action while another release of it downloads: the download
  // writes to the downloads folder, not to the game's. A check, a repair or a move of the game's own
  // files holds it instead, unless the game already runs.
  const busy = $derived(
    busyState([
      ownInstall ? { active: true, label: installStatusLabels(ownInstall.status), progress: ownInstall.progress, indeterminate: installIndeterminate(ownInstall) } : null,
      update && (update.state === 'updating' || update.state === 'update_downloading')
        ? { active: true, label: stepLabels(update.step ?? 'download'), progress: update.progress }
        : null,
      verifyState && !running && (verifyState.repairing || verifyState.running)
        ? { active: true, label: msg(verifyState.repairing ? 'games.mainActionRepairing' : 'games.mainActionVerifying'), progress: verifyState.progress }
        : null,
      ownMove && !running
        ? { active: true, label: msg('games.mainActionMoving'), progress: ownMove.totalBytes > 0 ? ownMove.copiedBytes / ownMove.totalBytes : 0 }
        : null,
      ownDownload && !installed ? { active: true, label: statusLabels(ownDownload.status), progress: ownDownload.progress } : null,
    ]),
  );

  const availableGroups = $derived(data.releaseGroups.filter((group) => group.release.availability !== 'removed'));
  const split = $derived(splitReleases(availableGroups));
  const pcGroups = $derived(split.pc);
  const consoleGroups = $derived(split.console);
  const downloadPool = $derived(pcGroups.length > 0 ? pcGroups : consoleGroups);
  const catalogCover = $derived(firstWebUrl(catalogGame?.coverUrl, metaView?.cover, localGame?.cover));

  const relatedShown = $derived(
    data.related
      .map((item) => ({ ...item, groups: item.groups.filter((group) => group.release.availability !== 'removed') }))
      .filter((item) => item.groups.length > 0),
  );
  const ownOnPc = $derived(onPC(info?.platforms));
  const relatedOk = $derived(data.relatedLoaded && !data.relatedLoading && !data.relatedFailed);
  const hasPcRelative = $derived(data.related.some((item) => item.onPc));
  const notOnPcKind = $derived(
    notOnPcBadge({ ownOnPc, relatedOk, hasPcRelative, installed, releaseCount: availableGroups.length }),
  );

  const primary = $derived(
    hubAction({
      installed,
      running,
      launchable: localGame ? canPlay(localGame) : true,
      updateAvailable,
      releaseCount: pcGroups.length,
      consoleCount: consoleGroups.length,
      consolePlatform: sharedPlatform(consoleGroups),
      releasesLoading: data.releasesLoading,
      relatedWithReleases: relatedShown.length > 0,
      notOnPc: !ownOnPc && relatedOk && !hasPcRelative,
      busy,
      terminalDownload: terminalDownload ? { status: terminalDownload.status } : null,
    }),
  );

  function openMatch(mode: 'find' | 'change') {
    matchMode = mode;
    matchOpen = true;
  }

  function onMenu(actionId: string) {
    if (actionId === 'saves-backups') {
      if (localGame) openSaveBackups(localGame.id);
    } else if (actionId === 'remove') {
      removeMode = 'library';
      removeOpen = true;
    } else if (actionId === 'uninstall') {
      removeMode = 'disk';
      removeOpen = true;
    } else if (actionId === 'meta-find') {
      openMatch('find');
    } else if (actionId === 'meta-change') {
      openMatch('change');
    } else if (actionId === 'meta-refresh') {
      data.refreshMeta();
    } else if (actionId === 'remove-download') {
      if (terminalDownload) {
        pending = { prompt: removeDownloadPrompt(terminalDownload.name), run: removeTerminalDownload };
      }
    } else if (actionId === 'discard-download') {
      if (terminalDownload) {
        pending = { prompt: discardDownloadPrompt(terminalDownload.name), run: discardTerminalDownload };
      }
    } else if (actionId === 'shortcut-create') {
      data.createDesktopShortcut();
    } else if (actionId === 'shortcut-remove') {
      data.removeDesktopShortcut();
    }
  }

  // The main action drives blocks of the overview; pressed on another tab it shows the overview first.
  async function toOverview() {
    if (activeTab === 'overview') return;
    activeTab = 'overview';
    await tick();
  }

  function openShot(index: number) {
    lightboxIndex = index;
    lightboxOpen = true;
  }

  let releasesRetrying = $state(false);
  let relatedRetrying = $state(false);
  let releasesBox = $state<HTMLElement>();

  let downloadModalOpen = $state(false);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let downloadSource = $state('');
  let downloadOrigin = $state<DownloadOrigin | undefined>(undefined);

  let installModalOpen = $state(false);
  let installModalDownloadId = $state<string | null>(null);

  async function retryTerminalDownload() {
    if (!terminalDownload) return;
    try {
      await resumeDownload(terminalDownload.id);
    } catch (err) {
      toast(installErrorText(err, msg('games.detailRetryDownloadError')), 'danger');
    }
  }

  function installFromTerminalDownload() {
    if (!terminalDownload) return;
    installModalDownloadId = terminalDownload.id;
    installModalOpen = true;
  }

  function leaveWithoutCard() {
    if (!localGame) navigate('library');
  }

  async function removeTerminalDownload() {
    if (!terminalDownload) return;
    try {
      await removeDownload(terminalDownload.id);
      leaveWithoutCard();
    } catch (err) {
      toast(installErrorText(err, msg('games.detailRemoveDownloadError')), 'danger');
    }
  }

  // Cancel purges the files only for a download that never completed:
  // manager.discard keeps a completed one, and DeleteData is the call that
  // removes its content.
  async function discardTerminalDownload() {
    if (!terminalDownload) return;
    const id = terminalDownload.id;
    const freesDisk = cancelFreesDisk(terminalDownload.status);
    try {
      await (freesDisk ? cancelDownload(id) : deleteDownloadData(id));
      leaveWithoutCard();
    } catch (err) {
      toast(installErrorText(err, msg('games.detailDiscardDownloadError')), 'danger');
    }
  }

  async function downloadRelease(group: ReleaseGroup, owner: ReleaseOwner = { gameId: canonicalId ?? '', title, cover: catalogCover }) {
    try {
      const request = await prepareReleaseDownload(group.release.id);
      downloadSource = request.uri;
      downloadOrigin = releaseDownloadOrigin(request, group.release, owner);
      downloadModalOpen = true;
    } catch (err) {
      toast(sourceErrorText(err, msg('games.detailPrepareDownloadError')), 'danger');
    }
  }

  function startDownload() {
    if (busy) return;
    if (downloadPool.length === 0) return;
    if (downloadPool.length === 1) {
      downloadRelease(downloadPool[0]);
      return;
    }
    pickerOpen = true;
  }

  const imageMissing = $derived(Boolean(localGame && $missingImages.has(localGame.id)));

  $effect(() => {
    if (emulated) void refreshMissingImages();
  });

  const missing = $derived(
    !data.catalogFailed &&
    isGameMissing({
      hasLocalGame: Boolean(localGame),
      hasCatalogGame: Boolean(catalogGame),
      catalogLoading: data.catalogLoading,
      hasOwnDownload: Boolean(anyOwnDownload),
    }),
  );
  const loadFailed = $derived((data.catalogFailed || data.catalogRetrying) && !localGame && !catalogGame && !anyOwnDownload);
  const gameShown = $derived(!missing && !loadFailed);
  const ambientId = $derived(pageAmbient(localGame?.id, canonicalId, gameShown));
  $effect(() => claimAmbient(ambientId));

  // The main button scrolls to or drives these sections, so a theme that hides
  // them must not leave the button doing nothing.
  const keepSections = $derived(
    primary.kind === 'update' ? ['update'] : primary.kind === 'other-versions' ? ['otherVersions'] : [],
  );
  const columns = $derived(gameSections($activeTheme?.layout, keepSections));
</script>

{#snippet column(ids: GameSection[])}
  <GameSections
    {ids} {data} {uid} {activeTab} {columns} {info} {title} {canonicalId} {releaseTitle} {localGame} {installed} {running}
    {managed} {external} {externalName} {emulated} {emulatedPlatform} {imageMissing} {verifyState} {update} {updateAvailable}
    {pcGroups} {availableGroups} {relatedShown} {downloadRelease}
    bind:summaryExpanded bind:updateCard bind:releasesRetrying bind:releasesBox bind:relatedRetrying bind:otherVersionsEl
  />
{/snippet}

{#snippet hero()}
  <GamePageHero
    {info} {title} {coverSrc} {heroSrc} {showHero} bind:heroFailed bind:titleHeading {localGame} {installed} {wasInstalled}
    {running} {notOnPcKind} {updateAvailable} {busy} {popOnce} {externalName} {emulated} {emulatedPlatform} {imageMissing}
  >
    {#snippet actions()}
      <GameActions
        {data} {primary} {busy} {localGame} {canonicalId} {installed} {wasInstalled} {external} {managed} {emulated}
        {updateIsRelease} {terminalDownload} {update} {updateCard} {otherVersionsEl} {toOverview} bind:statusOpen bind:tagsOpen {onMenu} {startDownload}
        {retryTerminalDownload} {installFromTerminalDownload}
      />
    {/snippet}
  </GamePageHero>
{/snippet}

<Page bleed={gameShown ? hero : undefined}>
  {#if missing}
    <EmptyState level={1} title={msg('games.detailMissingTitle')} description={msg('games.detailMissingDescription')}>
      {#snippet actions()}
        <Button onclick={() => navigate('library')}>{msg('games.detailBackToLibrary')}</Button>
      {/snippet}
    </EmptyState>
  {:else if loadFailed}
    <EmptyState level={1} title={msg('games.detailCatalogLoadError')} description={data.catalogFailure || undefined}>
      {#snippet actions()}
        <Button busy={data.catalogRetrying} onclick={data.retryCatalog}>{msg('common.retry')}</Button>
        <Button onclick={() => navigate('library')}>{msg('games.detailBackToLibrary')}</Button>
      {/snippet}
    </EmptyState>
  {:else}

    {#if tabs.length > 1}
      <div class="tabs-wrap" data-part="game-tabs">
        <Tabs {tabs} idPrefix="{uid}-game" label={title} bind:value={activeTab} />
      </div>
    {/if}

    <div class="body">
      <div class="main" data-part="game-main" {...tabs.length > 1 ? tabPanel(`${uid}-game`, activeTab) : {}}>
        {#if activeTab === 'screenshots'}
          {#if localGame && ownShots}
            <GameScreenshots gameId={localGame.id} {title} />
          {/if}
          <div
            class="shots"
            class:featured={shots.length >= 5}
            class:one={shots.length === 1}
            style:--cols={shotCols}
          >
            {#each shots as shot, index (shot.id)}
              <button class="shot" onclick={() => openShot(index)} aria-label={msg('games.detailOpenScreenshotLabel')}>
                <Artwork src={shot.url ?? ''} alt="" ratio="16 / 9" radius="var(--radius-sm)" />
              </button>
            {/each}
          </div>
          {@render column(columns.main)}
        {:else}
          <GameMetaNotice {data} {title} {titleHeading} {openMatch} />

          {@render column(columns.main)}
        {/if}
      </div>

      <aside class="side" data-part="game-side">
        {@render column(columns.side)}
      </aside>
    </div>

    <GameDialogs
      {data} {localGame} {installed} {emulated} {title} {canonicalId} {shots} {downloadPool} {downloadRelease} {matchMode} {downloadSource}
      {downloadOrigin} {installModalDownloadId} bind:statusOpen bind:tagsOpen bind:removeOpen bind:removeMode bind:pickerOpen
      bind:lightboxOpen bind:lightboxIndex bind:matchOpen bind:downloadModalOpen bind:installModalOpen
    />
  {/if}

  {#if pending}
    <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
  {/if}
</Page>

<style>
  .tabs-wrap {
    margin-bottom: var(--space-8);
  }

  .body {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 30rem;
    gap: var(--space-12);
    align-items: start;
  }

  .main {
    min-width: 0;
  }

  .main > :first-child {
    margin-top: 0;
  }

  .shots {
    display: grid;
    grid-template-columns: repeat(var(--cols, 3), minmax(0, 1fr));
    gap: var(--space-3);
  }

  .shots.one {
    max-width: 72rem;
  }

  .shot {
    display: block;
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: var(--radius-sm);
    overflow: hidden;
    transition:
      transform var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }

  .shot:hover {
    transform: translateY(-2px);
    opacity: 0.92;
  }

  .shots.featured .shot:first-child {
    grid-column: span 2;
    grid-row: span 2;
  }

  .side {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
  }

  @media (max-width: 1400px) {
    .body {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--space-8);
    }

    .side {
      flex-direction: row;
      flex-wrap: wrap;
      gap: var(--space-8);
      margin-top: var(--space-10);
    }

    .side > :global(*) {
      flex: 1 1 32rem;
    }

    .shots {
      grid-template-columns: repeat(auto-fill, minmax(28rem, 1fr));
    }

    .shots.featured {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 1200px) {
    .shots,
    .shots.featured {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .shots.featured .shot:first-child {
      grid-row: span 1;
    }
  }
</style>
