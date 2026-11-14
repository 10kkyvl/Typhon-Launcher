<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import ReleaseList from '../../lib/components/ReleaseList.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import type { GamePageData } from './gamePageData.svelte';
  import { relationKey } from '../../lib/game/releases';
  import { msg } from '../../lib/i18n';
  import type { RelatedReleases, ReleaseGroup } from '../../lib/services/sources';
  import { navigate } from '../../lib/stores/router';

  /** The other versions block: related games with releases, or why they failed to load. */
  let {
    data,
    relatedShown,
    availableGroups,
    downloadRelease,
    relatedRetrying = $bindable(),
    otherVersionsEl = $bindable(),
  }: {
    data: GamePageData;
    relatedShown: RelatedReleases[];
    availableGroups: ReleaseGroup[];
    downloadRelease: (group: ReleaseGroup) => void;
    relatedRetrying: boolean;
    otherVersionsEl: HTMLElement | null;
  } = $props();
</script>

<section class="section" bind:this={otherVersionsEl}>
  <h2 class="heading">{msg('games.otherVersionsHeading')}</h2>
  {#if data.relatedFailed || relatedRetrying}
    <Notice
      tone="danger"
      bind:retrying={relatedRetrying}
      retry={{ label: msg('games.catalogRetry'), run: data.retryRelated, within: () => otherVersionsEl }}
    >
      {msg('games.otherVersionsFailed')}
    </Notice>
  {:else}
    {#if availableGroups.length === 0 && !data.releasesLoading && !data.releasesFailed}
      <p class="muted">{msg('games.otherVersionsHint')}</p>
    {/if}
    {#each relatedShown as item (item.game.id)}
      {@const localId = item.gameId}
      <div class="related">
        <div class="related-head">
          {#if item.game.coverUrl}
            <span class="related-cover">
              <Artwork src={item.game.coverUrl} alt={item.game.title} ratio="3 / 4" radius="var(--radius-sm)" />
            </span>
          {/if}
          <div class="related-ident">
            {#if localId}
              <span class="related-title"><TextLink onclick={() => navigate('game', { id: localId })}>{item.game.title}</TextLink></span>
            {:else}
              <span class="related-title">{item.game.title}</span>
            {/if}
            <div class="related-meta">
              <StatusBadge kind="neutral" label={msg(relationKey(item.game.relation))} dot={false} />
              {#if item.game.releaseYear}<span class="related-year">{item.game.releaseYear}</span>{/if}
              {#if !item.onPc}<StatusBadge kind="neutral" label={msg('games.otherVersionsNotPc')} dot={false} />{/if}
            </div>
          </div>
        </div>
        <ReleaseList groups={item.groups} ondownload={downloadRelease} />
      </div>
    {/each}
  {/if}
</section>

<style>
  .section {
    margin-top: var(--space-10);
  }

  .related {
    margin-bottom: var(--space-8);
  }

  .related-head {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .related-cover {
    display: block;
    width: 6rem;
    flex-shrink: 0;
  }

  .related-ident {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }

  .related-title {
    font-size: var(--font-md);
    font-weight: 600;
    text-align: left;
  }

  .related-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .related-year {
    font-size: var(--font-sm);
    color: var(--text-muted);
  }

  .heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }
</style>
