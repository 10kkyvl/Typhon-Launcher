<script lang="ts">
  import {
    ArrowDownUp,
    ArrowUp,
    Bookmark,
    ChevronDown,
    ChevronRight,
    CircleCheck,
    Clock,
    FileDown,
    LayoutGrid,
    List,
    Play,
    SlidersHorizontal,
    Star,
    X,
  } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import DownloadItem from '../../lib/components/DownloadItem.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import GameHero from '../../lib/components/GameHero.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import { games } from '../../lib/mock/games';
  import { active, moveUp, queue, remove } from '../../lib/stores/downloads';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { libraryView } from '../../lib/stores/ui';
  import { bytesSize, gb, plural } from '../../lib/utils/format';

  type Filter = 'all' | 'favorites' | 'recent' | 'completed';

  let filter = $state<Filter>('all');
  let heroIndex = $state(0);

  const featured = games.filter((g) => g.installed && g.favorite).slice(0, 5);

  const filters: { id: Filter; label: string; icon?: typeof Star }[] = [
    { id: 'all', label: 'Все игры' },
    { id: 'favorites', label: 'Избранное', icon: Star },
    { id: 'recent', label: 'Недавние', icon: Clock },
    { id: 'completed', label: 'Пройденные', icon: CircleCheck },
  ];

  const sectionTitles: Record<Filter, string> = {
    all: 'Недавние игры',
    favorites: 'Избранное',
    recent: 'Недавние игры',
    completed: 'Пройденные',
  };

  const visibleGames = $derived.by(() => {
    switch (filter) {
      case 'favorites':
        return games.filter((g) => g.favorite);
      case 'recent':
        return games.filter((g) => g.lastPlayed !== null);
      case 'completed':
        return games.filter((g) => g.completed);
      default:
        return games;
    }
  });

  const hero = $derived(featured[heroIndex]);
  const activeDownload = $derived($active[0]);

  onMount(() => {
    const timer = setInterval(() => {
      heroIndex = (heroIndex + 1) % featured.length;
    }, 8000);
    return () => clearInterval(timer);
  });

  function playtime(hours: number) {
    return `${hours} ч`;
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
      <p class="hero-genres">{hero.genres.slice(0, 4).join(' · ')}</p>
      <p class="hero-tagline">{hero.tagline}</p>
      <div class="hero-actions">
        <Button variant="primary" size="lg" onclick={() => toast(`Запуск «${hero.title}»...`)}>
          <Play size="1.5rem" strokeWidth={2} fill="currentColor" />
          Играть
        </Button>
        <Button size="lg" onclick={() => navigate('game', { id: hero.id })}>Подробнее</Button>
        <IconButton label="В коллекцию" onclick={() => toast('Добавлено в коллекцию')}>
          <Bookmark size="1.8rem" strokeWidth={1.8} />
        </IconButton>
      </div>
    </GameHero>
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
  </div>
{/if}

<div class="toolbar">
  <div class="filters">
    <DropdownMenu
      align="left"
      items={[
        { id: 'all', label: 'Все игры' },
        { id: 'installed', label: 'Установленные' },
        { id: 'not-installed', label: 'Не установленные' },
      ]}
      onselect={() => (filter = 'all')}
    >
      {#snippet trigger({ open, toggle })}
        <button class="chip" class:selected={filter === 'all'} class:open onclick={toggle}>
          Все игры
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </button>
      {/snippet}
    </DropdownMenu>
    {#each filters.slice(1) as f (f.id)}
      <button class="chip" class:selected={filter === f.id} onclick={() => (filter = filter === f.id ? 'all' : f.id)}>
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
        { id: 'genre', label: 'По жанру' },
        { id: 'installed', label: 'Установленные' },
        { id: 'size', label: 'По размеру' },
      ]}
      onselect={() => toast('Фильтры недоступны в demo')}
    >
      {#snippet trigger({ open, toggle })}
        <button class="chip quiet" class:open onclick={toggle}>
          <SlidersHorizontal size="1.4rem" strokeWidth={1.8} />
          Фильтры
        </button>
      {/snippet}
    </DropdownMenu>
    <DropdownMenu
      items={[
        { id: 'alpha', label: 'По алфавиту' },
        { id: 'recent', label: 'По последнему запуску' },
        { id: 'playtime', label: 'По наигранному времени' },
        { id: 'size', label: 'По размеру' },
      ]}
      onselect={() => toast('Сортировка применена')}
    >
      {#snippet trigger({ open, toggle })}
        <button class="chip quiet" class:open onclick={toggle}>
          <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
          По алфавиту
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </button>
      {/snippet}
    </DropdownMenu>
  </div>
</div>

<section class="section">
  <div class="section-head">
    <h2>{sectionTitles[filter]}</h2>
    <button class="link" onclick={() => navigate('installed')}>
      Показать все
      <ChevronRight size="1.4rem" strokeWidth={1.8} />
    </button>
  </div>

  {#if visibleGames.length === 0}
    <EmptyState title="Здесь пока пусто" description="Игры, подходящие под этот фильтр, появятся здесь." />
  {:else if $libraryView === 'grid'}
    <div class="grid">
      {#each visibleGames as game (game.id)}
        <GameCard {game} meta={game.playtimeHours > 0 ? playtime(game.playtimeHours) : gb(game.sizeGb)} />
      {/each}
    </div>
  {:else}
    <div class="list">
      {#each visibleGames as game (game.id)}
        <button class="list-row" onclick={() => navigate('game', { id: game.id })}>
          <div class="list-thumb">
            <Artwork src={game.cover} alt={game.title} radius="var(--radius-xs)" />
          </div>
          <span class="list-title">{game.title}</span>
          <span class="list-meta">{game.genres.slice(0, 2).join(' · ')}</span>
          <span class="list-meta right">
            {game.playtimeHours > 0 ? playtime(game.playtimeHours) : gb(game.sizeGb)}
          </span>
        </button>
      {/each}
    </div>
  {/if}
</section>

{#if activeDownload || $queue.length > 0}
  <section class="section transfers">
    <div class="section-head">
      <h2>Загрузки</h2>
      <button class="link" onclick={() => navigate('downloads')}>
        Управление
        <ChevronRight size="1.4rem" strokeWidth={1.8} />
      </button>
    </div>
    <div class="transfers-grid">
      <div class="transfers-col">
        {#if activeDownload}
          <DownloadItem download={activeDownload} compact />
        {:else}
          <p class="muted">Нет активных загрузок</p>
        {/if}
      </div>
      <div class="transfers-col">
        {#if $queue.length === 0}
          <p class="muted">Очередь пуста</p>
        {:else}
          <div class="queue">
            {#each $queue.slice(0, 3) as q (q.id)}
              <div class="queue-row">
                <span class="queue-icon"><FileDown size="1.6rem" strokeWidth={1.8} /></span>
                <span class="queue-title">{q.name}</span>
                <span class="queue-size">{bytesSize(q.total)}</span>
                <div class="queue-actions">
                  <IconButton label="Выше" size="sm" onclick={() => moveUp(q.id)}>
                    <ArrowUp size="1.5rem" strokeWidth={1.8} />
                  </IconButton>
                  <IconButton label="Убрать" size="sm" onclick={() => remove(q.id)}>
                    <X size="1.5rem" strokeWidth={1.8} />
                  </IconButton>
                </div>
              </div>
            {/each}
            {#if $queue.length > 3}
              <p class="muted small">
                Ещё {$queue.length - 3} {plural($queue.length - 3, 'загрузка', 'загрузки', 'загрузок')} в очереди
              </p>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </section>
{/if}

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
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
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

  .link {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    font-size: var(--font-sm);
    color: var(--text-3);
    border-radius: var(--radius-sm);
    padding: 0.3rem 0.6rem;
    transition: color var(--dur) var(--ease);
  }

  .link:hover {
    color: var(--text);
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

  .transfers-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
    gap: var(--space-10);
    align-items: start;
  }

  .transfers-col {
    min-width: 0;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .muted.small {
    font-size: var(--font-xs);
    padding-top: 0.8rem;
  }

  .queue {
    display: flex;
    flex-direction: column;
  }

  .queue-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0.6rem 0;
  }

  .queue-row + .queue-row {
    border-top: 1px solid var(--border);
  }

  .queue-icon {
    display: inline-flex;
    color: var(--text-3);
    flex-shrink: 0;
  }

  .queue-title {
    flex: 1;
    min-width: 0;
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .queue-size {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .queue-actions {
    display: flex;
    gap: 2px;
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

  @media (max-width: 1240px) {
    .transfers-grid {
      grid-template-columns: 1fr;
      gap: var(--space-6);
    }
  }
</style>
