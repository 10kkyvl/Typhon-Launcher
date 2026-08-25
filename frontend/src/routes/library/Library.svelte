<script lang="ts">
  import {
    ArrowDownUp,
    ChevronDown,
    Clock,
    LayoutGrid,
    List,
    MonitorDown,
    Play,
    Square,
  } from '@lucide/svelte';
  import { onMount, untrack } from 'svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import GameHero from '../../lib/components/GameHero.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { playGame, stopGame, type LibraryGame } from '../../lib/services/library';
  import { getCatalogGames, type CatalogGame } from '../../lib/services/sources';
  import type { Download } from '../../lib/services/downloads';
  import { downloads, statusLabels } from '../../lib/stores/downloads';
  import { installedGames, libraryGames, runningGames } from '../../lib/stores/library';
  import { gameArt, requestArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { libraryView } from '../../lib/stores/ui';
  import { bytesSize, playtime, relativeDate } from '../../lib/utils/format';

  type Filter = 'all' | 'installed' | 'recent';
  type Sort = 'alpha' | 'recent' | 'playtime' | 'size';

  interface Entry {
    id: string;
    title: string;
    cover: string;
    hero: string;
    installed: boolean;
    playtimeSeconds: number;
    sizeBytes: number;
    lastPlayed: string | null;
    subtitle: string;
  }

  let filter = $state<Filter>('all');
  let sort = $state<Sort>('alpha');
  let heroIndex = $state(0);
  let catalogGames = $state<Record<string, CatalogGame>>({});

  const installedByGame = $derived.by(() => {
    const ids = new Set<string>();
    for (const game of $libraryGames) {
      if (game.canonicalGameId) ids.add(game.canonicalGameId);
    }
    return ids;
  });

  const downloaded = $derived.by(() => {
    const map = new Map<string, Download>();
    for (const item of $downloads) {
      const gameId = item.origin?.gameId;
      if (!gameId || installedByGame.has(gameId)) continue;
      const current = map.get(gameId);
      if (!current || Date.parse(item.addedAt) >= Date.parse(current.addedAt)) map.set(gameId, item);
    }
    return map;
  });

  async function loadCatalogGames(ids: string[]) {
    try {
      const games = await getCatalogGames(ids);
      if (games.length === 0) return;
      catalogGames = { ...catalogGames, ...Object.fromEntries(games.map((game) => [game.id, game])) };
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось загрузить данные игр', 'danger');
    }
  }

  $effect(() => {
    const ids = [...downloaded.keys()];
    untrack(() => {
      const missing = ids.filter((id) => !(id in catalogGames));
      if (missing.length > 0) loadCatalogGames(missing);
    });
  });

  $effect(() => {
    requestArt([
      ...downloaded.keys(),
      ...$libraryGames.map((game) => game.canonicalGameId).filter((cid): cid is string => Boolean(cid)),
    ]);
  });

  const filters: { id: Filter; label: string; icon?: typeof Clock }[] = [
    { id: 'all', label: 'Все' },
    { id: 'installed', label: 'Установленные', icon: MonitorDown },
    { id: 'recent', label: 'Недавние', icon: Clock },
  ];

  const sortLabels: Record<Sort, string> = {
    alpha: 'По алфавиту',
    recent: 'По последнему запуску',
    playtime: 'По наигранному времени',
    size: 'По размеру',
  };

  const sectionTitles: Record<Filter, string> = {
    all: 'Моя библиотека',
    installed: 'Установленные',
    recent: 'Недавние',
  };

  function installedEntry(game: LibraryGame): Entry {
    const bits: string[] = [];
    if (game.uninstalled) {
      bits.push('Не установлена');
    } else {
      if (game.version) bits.push(game.version);
      if (game.sizeBytes > 0) bits.push(bytesSize(game.sizeBytes));
    }
    const art = game.canonicalGameId ? $gameArt[game.canonicalGameId] : undefined;
    return {
      id: game.id,
      title: game.title,
      cover: art?.cover || game.cover,
      hero: art?.hero ?? '',
      installed: !game.uninstalled,
      playtimeSeconds: game.playtimeSeconds,
      sizeBytes: game.sizeBytes,
      lastPlayed: game.lastPlayed,
      subtitle: bits.join(' · '),
    };
  }

  function downloadedEntry(gameId: string, item: Download): Entry {
    const game = catalogGames[gameId];
    const bits: string[] = [];
    if (game?.releaseYear) bits.push(String(game.releaseYear));
    if (game?.developer) bits.push(game.developer);
    bits.push(statusLabels[item.status]);
    return {
      id: gameId,
      title: game?.title || item.name,
      cover: $gameArt[gameId]?.cover ?? '',
      hero: $gameArt[gameId]?.hero ?? '',
      installed: false,
      playtimeSeconds: 0,
      sizeBytes: item.total,
      lastPlayed: null,
      subtitle: bits.join(' · '),
    };
  }

  const entries = $derived.by(() => {
    const installed = $libraryGames.map(installedEntry);
    const rest = [...downloaded].map(([gameId, item]) => downloadedEntry(gameId, item));
    return [...installed, ...rest];
  });

  const visibleGames = $derived.by(() => {
    const filtered = entries.filter((entry) => {
      switch (filter) {
        case 'installed':
          return entry.installed;
        case 'recent':
          return entry.lastPlayed !== null;
        default:
          return true;
      }
    });

    return filtered.toSorted((a, b) => {
      switch (sort) {
        case 'recent':
          return time(b.lastPlayed) - time(a.lastPlayed) || a.title.localeCompare(b.title, 'ru');
        case 'playtime':
          return b.playtimeSeconds - a.playtimeSeconds || a.title.localeCompare(b.title, 'ru');
        case 'size':
          return b.sizeBytes - a.sizeBytes || a.title.localeCompare(b.title, 'ru');
        default:
          return a.title.localeCompare(b.title, 'ru');
      }
    });
  });

  function time(value: string | null) {
    if (!value) return 0;
    const parsed = Date.parse(value);
    return Number.isNaN(parsed) ? 0 : parsed;
  }

  const featured = $derived(
    $installedGames
      .map(installedEntry)
      .toSorted((a, b) => time(b.lastPlayed) - time(a.lastPlayed))
      .slice(0, 5),
  );

  const hero = $derived(featured[Math.min(heroIndex, Math.max(featured.length - 1, 0))]);

  onMount(() => {
    const timer = setInterval(() => {
      if (featured.length === 0) return;
      heroIndex = (heroIndex + 1) % featured.length;
    }, 8000);
    return () => clearInterval(timer);
  });

  function entryMeta(entry: Entry) {
    if (entry.sizeBytes > 0) return bytesSize(entry.sizeBytes);
    return entry.installed ? '' : 'Не установлена';
  }

  async function toggleRun(id: string) {
    try {
      if ($runningGames.has(id)) await stopGame(id);
      else await playGame(id);
    } catch (err) {
      toast(err instanceof Error && err.message ? err.message : 'Не удалось запустить игру', 'danger');
    }
  }
</script>

<PageHeader title="Библиотека">
  {#snippet actions()}
    <SegmentedControl
      bind:value={$libraryView}
      options={[
        { id: 'grid', label: 'Сетка' },
        { id: 'list', label: 'Список' },
      ]}
    >
      {#snippet item(option)}
        {#if option.id === 'grid'}
          <LayoutGrid size="1.6rem" strokeWidth={1.8} />
        {:else}
          <List size="1.6rem" strokeWidth={1.8} />
        {/if}
      {/snippet}
    </SegmentedControl>
  {/snippet}
</PageHeader>

{#if hero}
  <div class="hero-block">
    <GameHero src={hero.hero} alt={hero.title} ratio="3.4 / 1" minHeight="24rem" maxHeight="34rem">
      <h2 class="hero-title">{hero.title}</h2>
      {#if hero.subtitle}
        <p class="hero-genres">{hero.subtitle}</p>
      {/if}
      <p class="hero-tagline">
        {hero.lastPlayed ? `Последний запуск: ${relativeDate(hero.lastPlayed)}` : 'Ещё не запускалась'}
        {hero.playtimeSeconds > 0 ? ` · ${playtime(hero.playtimeSeconds)}` : ''}
      </p>
      <div class="hero-actions">
        <Button variant="primary" size="lg" onclick={() => toggleRun(hero.id)}>
          {#if $runningGames.has(hero.id)}
            <Square size="1.4rem" strokeWidth={2} fill="currentColor" />
            Остановить
          {:else}
            <Play size="1.5rem" strokeWidth={2} fill="currentColor" />
            Играть
          {/if}
        </Button>
        <Button size="lg" onclick={() => navigate('game', { id: hero.id })}>Подробнее</Button>
      </div>
    </GameHero>
    {#if featured.length > 1}
      <div class="hero-dots">
        {#each featured as g, i (g.id)}
          <button
            class="dot"
            class:on={i === heroIndex}
            aria-label={g.title}
            onclick={() => (heroIndex = i)}
          ></button>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<div class="toolbar">
  <div class="filters">
    {#each filters as f (f.id)}
      <button class="chip" class:selected={filter === f.id} onclick={() => (filter = f.id)}>
        {#if f.icon}
          <f.icon size="1.4rem" strokeWidth={1.8} />
        {/if}
        {f.label}
      </button>
    {/each}
  </div>
  <div class="toolbar-right">
    <DropdownMenu
      items={[
        { id: 'alpha', label: sortLabels.alpha },
        { id: 'recent', label: sortLabels.recent },
        { id: 'playtime', label: sortLabels.playtime },
        { id: 'size', label: sortLabels.size },
      ]}
      onselect={(id) => (sort = id as Sort)}
    >
      {#snippet trigger({ open, toggle })}
        <button class="chip quiet" class:open onclick={toggle}>
          <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
          {sortLabels[sort]}
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </button>
      {/snippet}
    </DropdownMenu>
  </div>
</div>

<section class="section">
  <div class="section-head">
    <h2>{sectionTitles[filter]}</h2>
  </div>

  {#if visibleGames.length === 0}
    <EmptyState
      title="Здесь пока пусто"
      description="Игры появятся тут после установки или загрузки. Каталог источников — в разделе «Все игры»."
    />
  {:else if $libraryView === 'grid'}
    <div class="grid">
      {#each visibleGames as entry (entry.id)}
        <GameCard
          id={entry.id}
          title={entry.title}
          cover={entry.cover}
          installed={entry.installed}
          running={$runningGames.has(entry.id)}
          meta={entryMeta(entry)}
          onplay={() => toggleRun(entry.id)}
        />
      {/each}
    </div>
  {:else}
    <div class="list">
      {#each visibleGames as entry (entry.id)}
        <button class="list-row" onclick={() => navigate('game', { id: entry.id })}>
          <div class="list-thumb">
            <Artwork src={entry.cover} alt={entry.title} radius="var(--radius-xs)" />
          </div>
          <span class="list-title">{entry.title}</span>
          <span class="list-meta">{entry.subtitle}</span>
          <span class="list-meta right">{entryMeta(entry)}</span>
        </button>
      {/each}
    </div>
  {/if}
</section>

<style>
  .hero-block {
    position: relative;
    margin-bottom: var(--space-8);
  }

  .hero-title {
    font-size: var(--font-hero);
    font-weight: 600;
    line-height: 1.05;
    letter-spacing: var(--tracking-title);
    text-shadow: 0 2px 1.8rem rgba(0, 0, 0, 0.5);
  }

  .hero-genres {
    margin-top: 1rem;
    font-size: var(--font-sm);
    color: rgba(255, 255, 255, 0.7);
    text-shadow: 0 1px 0.6rem rgba(0, 0, 0, 0.5);
  }

  .hero-tagline {
    margin-top: 1.2rem;
    max-width: 46rem;
    font-size: var(--font-md);
    line-height: 1.5;
    color: rgba(238, 242, 246, 0.85);
    text-shadow: 0 1px 0.8rem rgba(0, 0, 0, 0.5);
  }

  .hero-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-5);
  }

  .hero-dots {
    position: absolute;
    right: 2rem;
    bottom: 1.8rem;
    display: flex;
    gap: 0.6rem;
  }

  .dot {
    width: 1.8rem;
    height: 0.3rem;
    border-radius: var(--cut) 0.3rem 0.3rem var(--cut);
    background: rgba(255, 255, 255, 0.25);
    transition: background var(--dur) var(--ease);
  }

  .dot:hover {
    background: rgba(255, 255, 255, 0.5);
  }

  .dot.on {
    background: rgba(255, 255, 255, 0.9);
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-5);
    flex-wrap: wrap;
  }

  .filters,
  .toolbar-right {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: var(--control-sm);
    padding: 0 1.1rem;
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .chip :global(svg) {
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  .chip:hover,
  .chip.open {
    background: var(--hover);
    color: var(--text);
  }

  .chip.selected {
    background: var(--hover-strong);
    color: var(--text);
    border-radius: var(--cut) var(--radius-md) var(--radius-md) var(--radius-md);
  }

  .chip.selected :global(svg),
  .chip:hover :global(svg) {
    color: var(--text-2);
  }

  .chip.quiet {
    color: var(--text-3);
  }

  .section {
    margin-bottom: var(--space-10);
  }

  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .section-head h2 {
    font-size: var(--font-xl);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--space-6) var(--space-5);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .list-row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 0.8rem 0.8rem;
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .list-row + .list-row {
    border-top: 1px solid var(--border);
  }

  .list-row:hover {
    background: var(--hover);
  }

  .list-thumb {
    width: 3.2rem;
    height: 4.2rem;
    flex-shrink: 0;
    border-radius: var(--radius-xs);
    overflow: hidden;
  }

  .list-title {
    flex: 1;
    min-width: 0;
    font-size: var(--font-md);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .list-meta {
    font-size: var(--font-sm);
    color: var(--text-3);
    white-space: nowrap;
  }

  .list-meta.right {
    min-width: 6rem;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  @media (min-width: 2200px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    }
  }

  @media (max-width: 1400px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(14.5rem, 1fr));
    }
  }
</style>
