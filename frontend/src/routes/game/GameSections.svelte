<script lang="ts">
  import AchievementsCard from '../../lib/components/AchievementsCard.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import ReferenceLinks from '../../lib/components/ReferenceLinks.svelte';
  import ReleaseList from '../../lib/components/ReleaseList.svelte';
  import SimilarReleases from '../../lib/components/SimilarReleases.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import SteamExtras from '../../lib/components/SteamExtras.svelte';
  import StoreLinks from '../../lib/components/StoreLinks.svelte';
  import SystemRequirements from '../../lib/components/SystemRequirements.svelte';
  import UpdateCard from '../../lib/components/UpdateCard.svelte';
  import GameFriendsPanel from './GameFriendsPanel.svelte';
  import GameInstall from './GameInstall.svelte';
  import GameNotes from './GameNotes.svelte';
  import GameOtherVersions from './GameOtherVersions.svelte';
  import GameReviews from './GameReviews.svelte';
  import ReviewSummary from './ReviewSummary.svelte';
  import type { GamePageData } from './gamePageData.svelte';
  import { referenceKinds, steamFacts } from '../../lib/game/extras';
  import { familyIds, showSimilar } from '../../lib/game/similar';
  import { facts, joinLimited, orderPlatforms, summaryView, tagList } from '../../lib/game/view';
  import { msg } from '../../lib/i18n';
  import { genreLabel, themeLabel } from '../../lib/metadata/labels';
  import type { LibraryGame } from '../../lib/services/library';
  import { openReferenceLink, openStoreLink, type ReferenceKind, type StoreId } from '../../lib/services/metadata';
  import type { CatalogGame, RelatedReleases, ReleaseGroup } from '../../lib/services/sources';
  import type { Update, VerifyState } from '../../lib/services/updates';
  import { settings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { GAME_MAIN_SECTIONS, type GameSection } from '../../lib/theme/layout';
  import { metadataErrorText } from '../../lib/metadata/metadataErrors';
  import { formatCount, numericDate } from '../../lib/utils/format';

  /** One column of the game page: the layout's blocks in the order the theme gives them. */
  let {
    ids,
    data,
    uid,
    activeTab,
    columns,
    info,
    title,
    canonicalId,
    releaseTitle,
    localGame,
    installed,
    running,
    managed,
    external,
    externalName,
    emulated,
    emulatedPlatform,
    imageMissing,
    verifyState,
    update,
    updateAvailable,
    pcGroups,
    availableGroups,
    relatedShown,
    downloadRelease,
    summaryExpanded = $bindable(),
    updateCard = $bindable(),
    releasesRetrying = $bindable(),
    releasesBox = $bindable(),
    relatedRetrying = $bindable(),
    otherVersionsEl = $bindable(),
  }: {
    ids: GameSection[];
    data: GamePageData;
    uid: string;
    activeTab: string;
    columns: { main: GameSection[]; side: GameSection[] };
    info: CatalogGame | null;
    title: string;
    canonicalId: string | undefined;
    releaseTitle: string | undefined;
    localGame: LibraryGame | undefined;
    installed: boolean;
    running: boolean;
    managed: boolean;
    external: boolean;
    externalName: string;
    emulated: boolean;
    emulatedPlatform: string;
    imageMissing: boolean;
    verifyState: VerifyState | undefined;
    update: Update | undefined;
    updateAvailable: boolean;
    pcGroups: ReleaseGroup[];
    availableGroups: ReleaseGroup[];
    relatedShown: RelatedReleases[];
    downloadRelease: (group: ReleaseGroup) => void;
    summaryExpanded: boolean;
    updateCard: UpdateCard | undefined;
    releasesRetrying: boolean;
    releasesBox: HTMLElement | undefined;
    relatedRetrying: boolean;
    otherVersionsEl: HTMLElement | null;
  } = $props();

  const showUpdateCard = $derived(
    Boolean(
      update &&
        (update.availability.available ||
          update.canRollback ||
          update.state === 'updating' ||
          update.state === 'update_downloading' ||
          update.state === 'update_failed'),
    ),
  );

  const releaseDateLabel = $derived.by(() => {
    if (!info) return '';
    if (info.releaseDate) {
      const date = new Date(info.releaseDate);
      if (!Number.isNaN(date.getTime())) return numericDate(date);
    }
    return info.releaseYear ? String(info.releaseYear) : '';
  });

  const platforms = $derived(orderPlatforms(info?.platforms));

  const tags = $derived(tagList([info?.genres?.map(genreLabel), info?.themes?.map(themeLabel)]));
  const summary = $derived(summaryView(info?.summary ?? '', summaryExpanded));

  const gameFacts = $derived(
    facts([
      { label: msg('games.detailFactReleaseDate'), value: releaseDateLabel },
      { label: msg('games.detailFactDeveloper'), value: info?.developer },
      { label: msg('games.detailFactPublisher'), value: info?.publisher },
      { label: msg('games.detailFactPlatforms'), value: joinLimited(platforms, 4), full: platforms.join(', ') },
      ...steamFacts(data.metaView?.steam, msg, formatCount),
    ]),
  );

  const sysreqRevision = $derived(
    data.metaView?.steam
      ? `${data.metaView.steam.appId}:${data.metaView.steam.requirements?.minimum?.length ?? 0}:${data.metaView.steam.requirements?.recommended?.length ?? 0}`
      : '',
  );
  const references = $derived(referenceKinds(data.savePaths, Boolean(canonicalId && title)));

  const similarShown = $derived(
    showSimilar({
      releasesLoading: data.releasesLoading,
      releasesFailed: data.releasesFailed,
      releaseCount: pcGroups.length,
      relatedSettled: !data.relatedLoading && (data.relatedLoaded || data.relatedFailed),
      hasGameId: Boolean(canonicalId),
    }),
  );
  const similarFamily = $derived(data.relatedFailed ? [] : familyIds(data.related, canonicalId ?? ''));

  async function openStore(store: StoreId) {
    if (!canonicalId) return;
    try {
      await openStoreLink(canonicalId, store);
    } catch (err) {
      toast(metadataErrorText(err, msg('errMetadata.fallback')), 'danger');
    }
  }

  async function openReference(kind: ReferenceKind, appId = 0) {
    if (!canonicalId) return;
    try {
      await openReferenceLink(canonicalId, kind, appId);
    } catch (err) {
      toast(metadataErrorText(err, msg('games.detailDlcOpenError')), 'danger');
    }
  }

  function scrollToReviews() {
    document.getElementById('reviews')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  const reviewsShown = $derived(columns.main.includes('reviews') || columns.side.includes('reviews'));

  const isMainSection = (id: GameSection) => (GAME_MAIN_SECTIONS as readonly string[]).includes(id);
  const moved = (id: GameSection) => columns.main.includes(id) !== isMainSection(id);
  const part = (id: GameSection) => ({ 'data-part': 'game-section', 'data-id': id });
</script>

{#snippet sec_summary()}
  {#if summary.text || !title || data.metaState === 'searching'}
    <div class="gsec" class:moved={moved('summary')} {...part('summary')}>
      {#if summary.text}
        <p class="summary" id="{uid}-summary">{summary.text}</p>
        {#if summary.expandable}
          <div class="more">
            <Button
              variant="ghost"
              size="sm"
              aria-expanded={summaryExpanded}
              aria-controls="{uid}-summary"
              onclick={() => (summaryExpanded = !summaryExpanded)}
            >
              {summaryExpanded ? msg('games.detailCollapseSummary') : msg('games.detailExpandSummary')}
            </Button>
          </div>
        {/if}
      {:else if !title || data.metaState === 'searching'}
        <div aria-hidden="true">
          <div class="skeleton line"></div>
          <div class="skeleton line"></div>
          <div class="skeleton line short"></div>
        </div>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet sec_tags()}
  {#if tags.length > 0}
    <div class="gsec" class:moved={moved('tags')} {...part('tags')}>
      {#if tags.length > 0}
        <div class="tags">
          {#each tags as tag (tag)}
            <StatusBadge kind="neutral" label={tag} dot={false} />
          {/each}
        </div>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet sec_links()}
  {#if true}
    <div class="gsec" class:moved={moved('links')} {...part('links')}>
      <div class="stores">
        <StoreLinks links={data.metaView?.storeLinks} onopen={openStore} />
        <ReferenceLinks kinds={references} onopen={(kind) => openReference(kind)} />
      </div>
    </div>
  {/if}
{/snippet}

{#snippet sec_update()}
  {#if showUpdateCard && update}
    <div class="gsec" class:moved={moved('update')} {...part('update')}>
      {#if showUpdateCard && update}
        <section class="section">
          <UpdateCard bind:this={updateCard} {update} {running} />
        </section>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet sec_install()}
  {#if installed && localGame}
    <div class="gsec" class:moved={moved('install')} {...part('install')}>
      {#if installed && localGame}
        <GameInstall
          {data}
          {uid}
          {localGame}
          {installed}
          {managed}
          {external}
          {externalName}
          {emulated}
          {emulatedPlatform}
          {imageMissing}
          {verifyState}
          {running}
        />
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet releasesFailedNotice()}
  {#if data.releasesFailed || releasesRetrying}
    <Notice
      tone="danger"
      bind:retrying={releasesRetrying}
      retry={{ label: msg('games.catalogRetry'), run: data.retryReleases, within: () => releasesBox }}
    >
      {msg('games.detailReleasesFailed')}
    </Notice>
  {/if}
{/snippet}

{#snippet sec_requirements(withReleases: boolean)}
  {#if (data.catalogGame || localGame) && managed}
    <div class="gsec" class:moved={moved('requirements')} {...part('requirements')}>
      <section class="section">
        <h2 class="heading">{msg('games.detailAvailableDownloadsHeading')}</h2>
        {#if canonicalId}
          <SystemRequirements gameId={canonicalId} installPath={$settings?.gamesPath ?? ''} {installed} revision={sysreqRevision} />
        {/if}
        {#if withReleases}
          {@render releasesFailedNotice()}
          {#if !data.releasesLoading && !data.releasesFailed && data.releaseGroups.length === 0}<EmptyState size="inline" title={msg('games.detailNoReleases')} />{/if}
          <div bind:this={releasesBox}>
            <ReleaseList
              groups={data.releaseGroups}
              loading={data.releasesLoading}
              currentReleaseId={localGame?.releaseId ?? ''}
              targetReleaseId={updateAvailable ? (update?.availability.targetReleaseId ?? '') : ''}
              updateKind={updateAvailable ? (update?.availability.kind ?? 'none') : 'none'}
              ondownload={downloadRelease}
            />
          </div>
        {/if}
      </section>
    </div>
  {/if}
{/snippet}

{#snippet sec_releases()}
  {#if (data.catalogGame || localGame) && managed}
    <div class="gsec" class:moved={moved('releases')} {...part('releases')}>
      <section class="section">
        <h2 class="heading">{msg('games.detailAvailableDownloadsHeading')}</h2>
        {@render releasesFailedNotice()}
        {#if !data.releasesLoading && !data.releasesFailed && data.releaseGroups.length === 0}<EmptyState size="inline" title={msg('games.detailNoReleases')} />{/if}
        <div bind:this={releasesBox}>
          <ReleaseList
            groups={data.releaseGroups}
            loading={data.releasesLoading}
            currentReleaseId={localGame?.releaseId ?? ''}
            targetReleaseId={updateAvailable ? (update?.availability.targetReleaseId ?? '') : ''}
            updateKind={updateAvailable ? (update?.availability.kind ?? 'none') : 'none'}
            ondownload={downloadRelease}
          />
        </div>
      </section>
    </div>
  {/if}
{/snippet}

{#snippet sec_similar()}
  {#if (data.catalogGame || localGame) && managed && similarShown && canonicalId}
    <div class="gsec" class:moved={moved('similar')} {...part('similar')}>
      {#if similarShown && canonicalId}
        <SimilarReleases
          gameId={canonicalId}
          familyIds={similarFamily}
          onlinked={() => {
            data.loadReleases(canonicalId, releaseTitle);
            data.loadRelated(canonicalId);
          }}
        />
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet sec_otherVersions()}
  {#if (data.catalogGame || localGame) && managed && (relatedShown.length > 0 || ((data.relatedFailed || relatedRetrying) && availableGroups.length === 0))}
    <div class="gsec" class:moved={moved('otherVersions')} {...part('otherVersions')}>
      {#if relatedShown.length > 0 || ((data.relatedFailed || relatedRetrying) && availableGroups.length === 0)}
        <GameOtherVersions {data} {relatedShown} {availableGroups} {downloadRelease} bind:relatedRetrying bind:otherVersionsEl />
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet sec_notes()}
  {#if localGame}
    <div class="gsec" class:moved={moved('notes')} {...part('notes')}>
      {#if localGame}
        {#key localGame.id}
          <GameNotes gameId={localGame.id} stored={localGame.notes ?? ''} />
        {/key}
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet sec_steamExtras()}
  {#if true}
    <div class="gsec" class:moved={moved('steamExtras')} {...part('steamExtras')}>
      <SteamExtras steam={data.metaView?.steam} onopendlc={(appId) => openReference('steamdlc', appId)} />
    </div>
  {/if}
{/snippet}

{#snippet sec_reviews()}
  {#if !emulated}
    <div class="gsec" class:moved={moved('reviews')} {...part('reviews')}>
      <GameReviews canonicalGameId={canonicalId ?? ''} />
    </div>
  {/if}
{/snippet}

{#snippet sec_about()}
  {#if gameFacts.length > 0}
    <div class="gsec" class:moved={moved('about')} {...part('about')}>
      {#if gameFacts.length > 0}
        <Card title={msg('games.detailAboutTitle')} surface="panel" padding="md-even">
          <dl class="facts about">
            {#each gameFacts as fact (fact.label)}
              <div class="fact">
                <dt>{fact.label}</dt>
                <dd class:mono={fact.mono} title={fact.full ?? undefined}>{fact.value}</dd>
              </div>
            {/each}
          </dl>
        </Card>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet sec_achievements()}
  {#if localGame}
    <div class="gsec" class:moved={moved('achievements')} {...part('achievements')}>
      {#if localGame}
        <AchievementsCard gameId={localGame.id} />
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet sec_reviewSummary()}
  {#if !emulated}
    <div class="gsec" class:moved={moved('reviewSummary')} {...part('reviewSummary')}>
      <ReviewSummary canonicalGameId={canonicalId ?? ''} onopen={reviewsShown ? scrollToReviews : undefined} />
    </div>
  {/if}
{/snippet}

{#snippet sec_friends()}
  {#if !emulated}
    <div class="gsec" class:moved={moved('friends')} {...part('friends')}>
      <GameFriendsPanel canonicalGameId={canonicalId ?? ''} />
    </div>
  {/if}
{/snippet}

{#each ids as id, i (id)}
  {#if !(id === 'releases' && ids[i - 1] === 'requirements') && (activeTab !== 'screenshots' || !isMainSection(id))}
    {#if id === 'summary'}{@render sec_summary()}
    {:else if id === 'tags'}{@render sec_tags()}
    {:else if id === 'links'}{@render sec_links()}
    {:else if id === 'update'}{@render sec_update()}
    {:else if id === 'install'}{@render sec_install()}
    {:else if id === 'requirements'}{@render sec_requirements(ids[i + 1] === 'releases')}
    {:else if id === 'releases'}{@render sec_releases()}
    {:else if id === 'similar'}{@render sec_similar()}
    {:else if id === 'otherVersions'}{@render sec_otherVersions()}
    {:else if id === 'notes'}{@render sec_notes()}
    {:else if id === 'steamExtras'}{@render sec_steamExtras()}
    {:else if id === 'reviews'}{@render sec_reviews()}
    {:else if id === 'about'}{@render sec_about()}
    {:else if id === 'achievements'}{@render sec_achievements()}
    {:else if id === 'reviewSummary'}{@render sec_reviewSummary()}
    {:else if id === 'friends'}{@render sec_friends()}
    {/if}
  {/if}
{/each}

<style>
  /* The page's column rules, for the blocks this component draws inside its `.main` and `.side`. */
  :global(.main) > :first-child,
  :global(.main) > .gsec:first-child > :global(:first-child) {
    margin-top: 0;
  }

  .gsec:empty {
    display: none;
  }

  :global(.main) > .gsec.moved:not(:first-child) {
    max-width: 60rem;
    margin-top: var(--space-8);
  }

  :global(.main) > .gsec.moved {
    max-width: 60rem;
  }

  :global(.side) > .gsec {
    min-width: 0;
  }

  :global(.side) > .gsec.moved > :global(:first-child) {
    margin-top: 0;
  }

  :global(.side) > .gsec.moved {
    overflow-x: auto;
    overflow-wrap: anywhere;
  }

  .summary {
    max-width: var(--prose-max);
    font-size: var(--font-md);
    line-height: 1.65;
    color: var(--text-2);
    white-space: pre-line;
  }

  .more {
    margin-top: 1rem;
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
    margin-top: var(--space-5);
  }

  .stores {
    margin-top: var(--space-5);
  }

  .section {
    margin-top: var(--space-10);
  }

  .heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  .facts {
    display: flex;
    flex-direction: column;
  }

  .fact {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.9rem 0;
    border-top: 1px solid var(--border);
  }

  .about .fact:last-child {
    padding-bottom: 0;
  }

  dt {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  dd {
    font-size: var(--font-sm);
    color: var(--text);
    min-width: 0;
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    font-variant-numeric: tabular-nums;
  }

  dd.mono {
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .skeleton {
    background: var(--surface-3);
    animation: loading-breathe 1.6s ease-in-out 3;
    border-radius: var(--radius-sm);
  }

  .line {
    height: 1.6rem;
    margin-bottom: 1rem;
    max-width: var(--prose-max);
  }

  .line.short {
    width: 40%;
  }
</style>
