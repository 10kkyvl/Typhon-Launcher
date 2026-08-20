<script lang="ts">
  import {
    ArrowDownUp,
    ArrowUp,
    Bookmark,
    ChevronDown,
    ChevronRight,
    CircleCheck,
    Clock,
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
  import GameCard from '../../lib/components/GameCard.svelte';
  import GameHero from '../../lib/components/GameHero.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import { games, gameById } from '../../lib/mock/games';
  import { active, moveInQueue, queue, removeFromQueue } from '../../lib/stores/downloads';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { libraryView } from '../../lib/stores/ui';
  import { gb, plural } from '../../lib/utils/format';

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

<h1 class="page-title">Библиотека</h1>

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
        <button class="pill primary-pill" class:open onclick={toggle}>
          Все игры
          <ChevronDown size="1.5rem" strokeWidth={1.8} />
        </button>
      {/snippet}
    </DropdownMenu>
    {#each filters.slice(1) as f (f.id)}
      <button class="pill" class:selected={filter === f.id} onclick={() => (filter = filter === f.id ? 'all' : f.id)}>
        {#if f.icon}
          <f.icon size="1.5rem" strokeWidth={1.8} />
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
        <button class="pill" class:open onclick={toggle}>
          <SlidersHorizontal size="1.5rem" strokeWidth={1.8} />
          Фильтры
          <ChevronDown size="1.5rem" strokeWidth={1.8} />
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
        <button class="pill" class:open onclick={toggle}>
          <ArrowDownUp size="1.5rem" strokeWidth={1.8} />
          Сортировка: По алфавиту
          <ChevronDown size="1.5rem" strokeWidth={1.8} />
        </button>
      {/snippet}
    </DropdownMenu>
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
  </div>
</div>

{#if hero}
  <div class="hero-block">
    <GameHero src={hero.hero} alt={hero.title} ratio="3.4 / 1" minHeight="24rem">
      <h2 class="hero-title">{hero.title}</h2>
      <div class="hero-genres">
        {#each hero.genres.slice(0, 4) as genre (genre)}
          <span class="genre">{genre}</span>
        {/each}
      </div>
      <p class="hero-tagline">{hero.tagline}</p>
      <div class="hero-actions">
        <Button variant="primary" size="lg" onclick={() => toast(`Запуск «${hero.title}»...`)}>
          <Play size="1.6rem" strokeWidth={2} fill="currentColor" />
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

<section class="section">
  <div class="section-head">
    <h2>{sectionTitles[filter]}</h2>
    <button class="link" onclick={() => navigate('installed')}>
      Показать все
      <ChevronRight size="1.5rem" strokeWidth={1.8} />
    </button>
  </div>

  {#if $libraryView === 'grid'}
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
            <Artwork src={game.cover} alt={game.title} radius="0.6rem" />
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

<div class="bottom-row">
  <section class="panel">
    <div class="panel-head">
      <h3>Активная загрузка</h3>
    </div>
    {#if activeDownload}
      <DownloadItem download={activeDownload} compact />
    {:else}
      <p class="panel-empty">Нет активных загрузок</p>
    {/if}
  </section>

  <section class="panel">
    <div class="panel-head">
      <h3>Очередь загрузки ({$queue.length})</h3>
    </div>
    {#if $queue.length === 0}
      <p class="panel-empty">Очередь пуста</p>
    {:else}
      <div class="queue">
        {#each $queue.slice(0, 3) as q (q.id)}
          {@const game = gameById(q.gameId)}
          <div class="queue-row">
            <div class="queue-thumb">
              <Artwork src={game?.hero ?? ''} alt={game?.title ?? ''} radius="0.6rem" />
            </div>
            <span class="queue-title">{game?.title}</span>
            <span class="queue-size">{gb(q.sizeGb)}</span>
            <span class="queue-state">В очереди</span>
            <div class="queue-actions">
              <IconButton label="Выше" size="sm" onclick={() => moveInQueue(q.id, -1)}>
                <ArrowUp size="1.5rem" strokeWidth={1.8} />
              </IconButton>
              <IconButton label="Убрать" size="sm" onclick={() => removeFromQueue(q.id)}>
                <X size="1.5rem" strokeWidth={1.8} />
              </IconButton>
            </div>
          </div>
        {/each}
      </div>
      <div class="panel-foot">
        <span class="panel-note">
          {$queue.length}
          {plural($queue.length, 'игра', 'игры', 'игр')} в очереди
        </span>
        <Button size="sm" onclick={() => navigate('downloads')}>Управление</Button>
      </div>
    {/if}
  </section>
</div>

<style>
  .page-title {
    font-size: var(--font-title);
    letter-spacing: -0.01em;
    margin: var(--space-4) 0 var(--space-5);
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-6);
    flex-wrap: wrap;
  }

  .filters,
  .toolbar-right {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: 0.8rem;
    height: 4rem;
    padding: 0 1.4rem;
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    background: var(--surface);
    font-size: 1.4rem;
    font-weight: 500;
    color: var(--text-2);
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .pill:hover,
  .pill.open {
    background: var(--surface-3);
    color: var(--text);
    border-color: var(--border-strong);
  }

  .pill.selected {
    background: var(--accent-subtle);
    border-color: rgba(104, 117, 232, 0.4);
    color: var(--accent-text);
  }

  .pill.primary-pill {
    background: var(--accent);
    border-color: transparent;
    color: #fff;
  }

  .pill.primary-pill:hover {
    background: var(--accent-hover);
  }

  .hero-block {
    position: relative;
    margin-bottom: var(--space-8);
  }

  .hero-title {
    font-size: clamp(3.2rem, 3vw, 4.4rem);
    font-weight: 600;
    letter-spacing: -0.01em;
    text-shadow: 0 2px 1.8rem rgba(0, 0, 0, 0.5);
  }

  .hero-genres {
    display: flex;
    gap: 0.8rem;
    margin-top: 1.4rem;
    flex-wrap: wrap;
  }

  .genre {
    padding: 0.4rem 1.2rem;
    border-radius: 0.8rem;
    background: rgba(10, 14, 19, 0.55);
    border: 1px solid rgba(255, 255, 255, 0.1);
    font-size: 1.3rem;
    color: var(--text-2);
    backdrop-filter: blur(0.4rem);
  }

  .hero-tagline {
    margin-top: 1.4rem;
    max-width: 42rem;
    font-size: 1.5rem;
    color: rgba(243, 245, 247, 0.85);
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
    gap: var(--space-3);
    margin-top: var(--space-5);
  }

  .hero-dots {
    position: absolute;
    right: 2rem;
    bottom: 1.6rem;
    display: flex;
    gap: 0.7rem;
  }

  .dot {
    width: 0.7rem;
    height: 0.7rem;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.28);
    transition:
      background var(--dur) var(--ease),
      transform var(--dur) var(--ease);
  }

  .dot:hover {
    background: rgba(255, 255, 255, 0.5);
  }

  .dot.on {
    background: var(--accent);
    transform: scale(1.25);
  }

  .section {
    margin-bottom: var(--space-8);
  }

  .section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .section-head h2 {
    font-size: 2rem;
  }

  .link {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 1.4rem;
    color: var(--text-3);
    border-radius: 0.7rem;
    padding: 0.4rem 0.8rem;
    transition: color var(--dur) var(--ease);
  }

  .link:hover {
    color: var(--text);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(18.5rem, 1fr));
    gap: var(--space-5);
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .list-row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: 0.8rem 1.2rem;
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .list-row:hover {
    background: rgba(255, 255, 255, 0.03);
  }

  .list-thumb {
    width: 3.4rem;
    height: 4.6rem;
    flex-shrink: 0;
    border-radius: 0.6rem;
    overflow: hidden;
  }

  .list-title {
    flex: 1;
    min-width: 0;
    font-size: 1.5rem;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .list-meta {
    font-size: 1.4rem;
    color: var(--text-3);
    white-space: nowrap;
  }

  .list-meta.right {
    min-width: 6rem;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  .bottom-row {
    display: grid;
    grid-template-columns: 1.4fr 1fr;
    gap: var(--space-5);
    align-items: start;
  }

  .panel {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-5);
  }

  .panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .panel-head h3 {
    font-size: 1.6rem;
    font-weight: 600;
  }

  .panel-empty {
    font-size: 1.4rem;
    color: var(--text-3);
    padding: var(--space-2) 0 var(--space-1);
  }

  .queue {
    display: flex;
    flex-direction: column;
  }

  .queue-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0.7rem 0;
  }

  .queue-row + .queue-row {
    border-top: 1px solid var(--border);
  }

  .queue-thumb {
    width: 5.6rem;
    aspect-ratio: 16 / 9;
    flex-shrink: 0;
    border-radius: 0.6rem;
    overflow: hidden;
  }

  .queue-title {
    flex: 1;
    min-width: 0;
    font-size: 1.4rem;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .queue-size {
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .queue-state {
    font-size: 1.3rem;
    color: var(--text-3);
    white-space: nowrap;
  }

  .queue-actions {
    display: flex;
    gap: 2px;
  }

  .panel-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
  }

  .panel-note {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  @media (max-width: 1400px) {
    .queue-state {
      display: none;
    }
  }

  @media (max-width: 1240px) {
    .bottom-row {
      grid-template-columns: 1fr;
    }
  }
</style>
