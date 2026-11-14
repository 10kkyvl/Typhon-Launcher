<script lang="ts">
  import { untrack } from 'svelte';
  import { Link2 } from '@lucide/svelte';
  import { platformBadge } from '../library/emulation';
  import { candidateLabel, relativeCandidate } from '../game/similar';
  import { confirmMatch, similarReleases, type SimilarRelease } from '../services/sources';
  import { sourceErrorText } from '../sources/sourceErrors';
  import { toast } from '../stores/toasts';
  import { metadataErrorText } from '../metadata/metadataErrors';
  import { focusHandOff } from '../utils/focusHandOff';
  import { bytesSize, relativeDate } from '../utils/format';
  import { msg } from '../i18n';
  import Button from './Button.svelte';
  import ConsoleTag from './ConsoleTag.svelte';
  import Notice from './Notice.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let {
    gameId,
    familyIds,
    onlinked,
  }: {
    gameId: string;
    familyIds: string[];
    onlinked?: () => void;
  } = $props();

  let items = $state<SimilarRelease[]>([]);
  let failed = $state(false);
  let retrying = $state(false);
  let linking = $state<string | null>(null);
  let token = 0;
  let section = $state<HTMLElement>();
  let list = $state<HTMLElement>();
  const rows: Record<string, HTMLElement | null> = {};

  const familyKey = $derived(familyIds.join('|'));

  // A retry keeps the failure on screen until its answer: the notice and its focused Retry stay.
  async function load(id: string, family: string[], retry = false) {
    const current = ++token;
    if (!retry) failed = false;
    try {
      const next = await similarReleases(id, family);
      if (current !== token) return;
      items = next;
      failed = false;
    } catch {
      if (current !== token) return;
      items = [];
      failed = true;
    }
  }

  async function retry(): Promise<boolean | undefined> {
    const id = gameId;
    await load(id, familyIds, true);
    if (id !== gameId) return undefined;
    return !failed;
  }

  $effect(() => {
    const id = gameId;
    familyKey;
    untrack(() => load(id, familyIds));
  });

  // A linked release leaves the list, and the list often leaves the page with it (the game has a
  // release of its own now): focus goes on to the next release's Link, else to the page's heading.
  async function link(item: SimilarRelease) {
    if (linking !== null) return;
    const id = item.release.id;
    const home = section?.closest('main') ?? null;
    const handOff = focusHandOff();
    const order = items.map((entry) => entry.release.id);
    linking = id;
    try {
      await confirmMatch(id, gameId);
      toast(msg('games.similarLinked'), 'success');
      onlinked?.();
      await load(gameId, familyIds);
    } catch (err) {
      toast(sourceErrorText(err, metadataErrorText(err, msg('games.blocks.similarLinkFailed'))), 'danger');
    } finally {
      linking = null;
    }
    await handOff(() => nextLink(order, id) ?? home?.querySelector<HTMLElement>('h1'));
  }

  function nextLink(order: string[], gone: string): HTMLElement | null {
    if (!list?.isConnected) return null;
    const left = new Set(items.map((entry) => entry.release.id));
    const after = order.slice(order.indexOf(gone) + 1).concat(order.slice(0, order.indexOf(gone)).reverse());
    const next = after.find((entry) => left.has(entry));
    return next ? (rows[next]?.querySelector<HTMLElement>('button') ?? null) : null;
  }
</script>

{#if failed || retrying || items.length > 0}
  <section class="section" bind:this={section}>
    <h2 class="heading">{msg('games.similarHeading')}</h2>
    {#if failed || retrying}
      <Notice tone="danger" compact bind:retrying retry={{ label: msg('games.catalogRetry'), run: retry, within: () => list }}>
        {msg('games.similarFailed')}
      </Notice>
    {:else}
      <p class="muted">{msg('games.similarHint')}</p>
      <div class="list" bind:this={list}>
        {#each items as item (item.release.id)}
          {@const note = relativeCandidate(item, gameId)}
          <div class="row" bind:this={rows[item.release.id]}>
            <div class="main">
              <span class="title" title={item.release.rawTitle}>{item.release.title || item.release.rawTitle}</span>
              {#if item.release.platform}
                <span class="note"><ConsoleTag platform={item.release.platform} /> {msg('games.similarPlatform', { platform: platformBadge(item.release.platform) })}</span>
              {/if}
              {#if note}
                <span class="note">{msg('games.similarCandidate', { title: candidateLabel(note) })}</span>
              {/if}
            </div>
            <span class="size">{bytesSize(item.release.size)}</span>
            <span class="source">{item.sourceName}</span>
            <span class="date">{relativeDate(item.release.uploadedAt)}</span>
            {#if item.release.matchStatus === 'review'}
              <StatusBadge kind="warning" label={msg('modals.sourceDetailsStatusReview')} plain />
            {/if}
            <Button size="sm" busy={linking === item.release.id} disabled={linking !== null && linking !== item.release.id} onclick={() => link(item)}>
              <Link2 size="1.4rem" strokeWidth={1.8} />
              {msg('games.similarLink')}
            </Button>
          </div>
        {/each}
      </div>
    {/if}
  </section>
{/if}

<style>
  .section {
    margin-top: var(--space-10);
  }

  .heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
    padding: 0.4rem 0;
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 1.1rem 0;
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    flex: 1.3;
  }

  .title {
    font-size: var(--font-sm);
    overflow-wrap: anywhere;
  }

  .note {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .size,
  .source,
  .date {
    font-size: var(--font-xs);
    color: var(--text-2);
    white-space: nowrap;
  }
</style>
