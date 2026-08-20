<script lang="ts">
  import {
    Bookmark,
    Calendar,
    Check,
    ChevronDown,
    ChevronRight,
    CircleCheck,
    Download,
    ExternalLink,
    FileCheck,
    FolderOpen,
    Gamepad2,
    Globe,
    Image,
    NotebookPen,
    Play,
    Settings,
    SquarePen,
    Terminal,
    Trash2,
    Trophy,
    UploadCloud,
    UserRound,
  } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameHero from '../../lib/components/GameHero.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { achievements, dlcs } from '../../lib/mock/achievements';
  import { gameById } from '../../lib/mock/games';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { gb } from '../../lib/utils/format';

  let { id }: { id: string } = $props();

  const game = $derived(gameById(id));
  const gameAchievements = $derived(achievements[id]);
  const gameDlcs = $derived(dlcs.filter((d) => d.gameId === id));

  let tab = $state('overview');
  let expanded = $state(false);
  let bookmarked = $state(false);

  const tabs = [
    { id: 'overview', label: 'Обзор' },
    { id: 'addons', label: 'Дополнения' },
    { id: 'achievements', label: 'Достижения' },
    { id: 'screenshots', label: 'Скриншоты' },
    { id: 'notes', label: 'Заметки' },
  ];

  const quickActions = [
    { id: 'settings', label: 'Параметры игры', sub: 'Графика, язык и др.', icon: Settings },
    { id: 'saves', label: 'Резервные копии', sub: 'Управление сохранениями', icon: UploadCloud },
    { id: 'verify', label: 'Проверить файлы', sub: 'Проверка целостности', icon: FileCheck },
    { id: 'folder', label: 'Открыть папку', sub: 'Папка с игрой', icon: FolderOpen },
    { id: 'args', label: 'Аргументы запуска', sub: 'Параметры командной строки', icon: Terminal },
    { id: 'uninstall', label: 'Удалить игру', sub: 'Освободить место', icon: Trash2, danger: true },
  ];

  function quickAction(actionId: string) {
    if (actionId === 'uninstall') toast('Удаление недоступно в demo', 'danger');
    else if (actionId === 'verify') toast('Проверка файлов запущена');
    else toast('Действие недоступно в demo');
  }
</script>

{#if !game}
  <EmptyState title="Игра не найдена" description="Возможно, она была удалена из библиотеки.">
    {#snippet actions()}
      <Button onclick={() => navigate('library')}>В библиотеку</Button>
    {/snippet}
  </EmptyState>
{:else}
  <nav class="breadcrumb">
    <button class="crumb" onclick={() => navigate('library')}>Библиотека</button>
    <ChevronRight size={14} strokeWidth={1.8} />
    <span class="crumb current">{game.title}</span>
  </nav>

  <GameHero src={game.hero} alt={game.title} ratio="2.9 / 1" minHeight="300px">
    <h1 class="title">{game.title}</h1>
    <div class="genres">
      {#each game.genres as genre (genre)}
        <span class="genre">{genre}</span>
      {/each}
    </div>
    <div class="meta">
      <span class="meta-item">
        <Calendar size={15} strokeWidth={1.8} />
        <span class="meta-label">Выпуск</span>
        {game.releaseDate}
      </span>
      <span class="meta-item">
        <UserRound size={15} strokeWidth={1.8} />
        <span class="meta-label">Разработчик</span>
        {game.developer}
      </span>
      <span class="meta-item">
        <Globe size={15} strokeWidth={1.8} />
        <span class="meta-label">Язык</span>
        {game.language}
      </span>
    </div>
    <p class="tagline">{game.tagline}</p>
    <div class="actions">
      {#if game.installed}
        <Button variant="primary" size="lg" onclick={() => toast(`Запуск «${game.title}»...`)}>
          <Play size={16} strokeWidth={2} fill="currentColor" />
          Играть
        </Button>
        <DropdownMenu
          align="left"
          items={[
            { id: 'update', label: 'Проверить обновления' },
            { id: 'shortcut', label: 'Создать ярлык' },
            { id: 'uninstall', label: 'Удалить игру', danger: true, separator: true },
          ]}
          onselect={quickAction}
        >
          {#snippet trigger({ toggle })}
            <span class="installed-group">
              <Button size="lg">
                <Check size={16} strokeWidth={2} />
                Установлено
              </Button>
              <Button size="lg" onclick={toggle}>
                <ChevronDown size={16} strokeWidth={1.8} />
              </Button>
            </span>
          {/snippet}
        </DropdownMenu>
      {:else}
        <Button variant="primary" size="lg" onclick={() => toast(`«${game.title}» добавлена в очередь загрузки`)}>
          <Download size={16} strokeWidth={2} />
          Установить
        </Button>
        <span class="size-hint">{gb(game.sizeGb)}</span>
      {/if}
      <IconButton label="В коллекцию" active={bookmarked} onclick={() => (bookmarked = !bookmarked)}>
        <Bookmark size={18} strokeWidth={1.8} fill={bookmarked ? 'currentColor' : 'none'} />
      </IconButton>
    </div>
  </GameHero>

  <div class="tabs-row">
    <Tabs {tabs} bind:value={tab} />
  </div>

  {#if tab === 'overview'}
    <div class="overview">
      <Card>
        <h3 class="card-title">О игре</h3>
        <p class="description">{game.description}</p>
        <div class="divider"></div>
        <dl class="props">
          <div class="prop">
            <dt>Режимы игры</dt>
            <dd>{game.modes.join(', ')}</dd>
          </div>
          <div class="prop">
            <dt>Поддержка контроллера</dt>
            <dd>{game.controllerSupport}</dd>
          </div>
          <div class="prop">
            <dt>Последнее обновление</dt>
            <dd>{game.lastUpdate}</dd>
          </div>
          {#if expanded}
            <div class="prop">
              <dt>Издатель</dt>
              <dd>{game.publisher}</dd>
            </div>
            <div class="prop">
              <dt>Версия</dt>
              <dd>{game.version}</dd>
            </div>
            <div class="prop">
              <dt>Размер</dt>
              <dd>{gb(game.sizeGb)}</dd>
            </div>
            {#if game.playtimeHours > 0}
              <div class="prop">
                <dt>Наиграно</dt>
                <dd>{game.playtimeHours} ч</dd>
              </div>
            {/if}
          {/if}
        </dl>
        <button class="expand" onclick={() => (expanded = !expanded)}>
          {expanded ? 'Показать меньше' : 'Показать больше'}
          <ChevronDown size={15} strokeWidth={1.8} style={expanded ? 'transform: rotate(180deg)' : ''} />
        </button>
      </Card>

      <Card>
        <div class="card-head">
          <h3 class="card-title">Достижения</h3>
          {#if gameAchievements}
            <button class="link" onclick={() => (tab = 'achievements')}>Показать все</button>
          {/if}
        </div>
        {#if gameAchievements}
          <div class="ach-summary">
            <div class="ach-badge">
              <Trophy size={22} strokeWidth={1.8} />
            </div>
            <div class="ach-count">
              <span class="ach-nums"><strong>{gameAchievements.earned}</strong> / {gameAchievements.total}</span>
              <span class="ach-label">Достижений получено</span>
            </div>
          </div>
          <ProgressBar value={gameAchievements.earned} max={gameAchievements.total} />
          <div class="ach-recent">
            <span class="ach-recent-label">Последнее достижение</span>
            {#each gameAchievements.recent.slice(0, 1) as a (a.name)}
              <div class="ach-item">
                <div class="ach-icon">
                  <Trophy size={16} strokeWidth={1.8} />
                </div>
                <div class="ach-text">
                  <span class="ach-name">{a.name}</span>
                  <span class="ach-desc">{a.description}</span>
                </div>
                <span class="ach-date">{a.date}</span>
              </div>
            {/each}
          </div>
        {:else}
          <p class="muted">Пока нет полученных достижений.</p>
        {/if}
      </Card>

      <Card>
        <div class="card-head">
          <h3 class="card-title">Установленные дополнения</h3>
          {#if gameDlcs.length > 0}
            <span class="muted small">
              {gameDlcs.filter((d) => d.installed).length} / {gameDlcs.length}
            </span>
          {/if}
        </div>
        {#if gameDlcs.length === 0}
          <p class="muted">У этой игры нет дополнений.</p>
        {:else}
          <div class="dlc-list">
            {#each gameDlcs as dlc (dlc.id)}
              <div class="dlc">
                <div class="dlc-text">
                  <span class="dlc-name">{dlc.name}</span>
                  <span class="dlc-kind">{dlc.kind}</span>
                </div>
                {#if dlc.installed}
                  <span class="dlc-state installed">
                    Установлено
                    <CircleCheck size={15} strokeWidth={1.8} />
                  </span>
                {:else}
                  <span class="dlc-state">Не установлено</span>
                {/if}
              </div>
            {/each}
          </div>
          <button class="link with-icon" onclick={() => (tab = 'addons')}>
            Управление дополнениями
            <ExternalLink size={14} strokeWidth={1.8} />
          </button>
        {/if}
      </Card>
    </div>

    <Card padding="var(--space-5) var(--space-6)">
      <h3 class="card-title qa-title">Быстрые действия</h3>
      <div class="quick-actions">
        {#each quickActions as action (action.id)}
          <button class="qa" class:danger={action.danger} onclick={() => quickAction(action.id)}>
            <action.icon size={19} strokeWidth={1.8} />
            <span class="qa-text">
              <span class="qa-label">{action.label}</span>
              <span class="qa-sub">{action.sub}</span>
            </span>
          </button>
        {/each}
      </div>
    </Card>
  {:else if tab === 'addons'}
    {#if gameDlcs.length === 0}
      <EmptyState title="Дополнений нет" description="У этой игры пока нет доступных дополнений.">
        {#snippet icon()}
          <Gamepad2 size={22} strokeWidth={1.8} />
        {/snippet}
      </EmptyState>
    {:else}
      <div class="addons">
        {#each gameDlcs as dlc (dlc.id)}
          <Card padding="var(--space-4) var(--space-5)">
            <div class="addon">
              <div class="dlc-text">
                <span class="dlc-name">{dlc.name}</span>
                <span class="dlc-kind">{dlc.kind}</span>
              </div>
              {#if dlc.installed}
                <StatusBadge kind="success" label="Установлено" dot={false} />
              {:else}
                <Button size="sm" onclick={() => toast(`«${dlc.name}» добавлено в очередь`)}>
                  <Download size={14} strokeWidth={1.8} />
                  Установить
                </Button>
              {/if}
            </div>
          </Card>
        {/each}
      </div>
    {/if}
  {:else if tab === 'achievements'}
    {#if gameAchievements}
      <Card>
        <div class="ach-summary">
          <div class="ach-badge">
            <Trophy size={22} strokeWidth={1.8} />
          </div>
          <div class="ach-count">
            <span class="ach-nums"><strong>{gameAchievements.earned}</strong> / {gameAchievements.total}</span>
            <span class="ach-label">Достижений получено</span>
          </div>
        </div>
        <ProgressBar value={gameAchievements.earned} max={gameAchievements.total} />
        <div class="ach-list">
          {#each gameAchievements.recent as a (a.name)}
            <div class="ach-item">
              <div class="ach-icon">
                <Trophy size={16} strokeWidth={1.8} />
              </div>
              <div class="ach-text">
                <span class="ach-name">{a.name}</span>
                <span class="ach-desc">{a.description}</span>
              </div>
              <span class="ach-date">{a.date}</span>
            </div>
          {/each}
        </div>
      </Card>
    {:else}
      <EmptyState title="Достижений пока нет" description="Запустите игру, чтобы начать получать достижения.">
        {#snippet icon()}
          <Trophy size={22} strokeWidth={1.8} />
        {/snippet}
      </EmptyState>
    {/if}
  {:else if tab === 'screenshots'}
    <EmptyState title="Скриншотов пока нет" description="Скриншоты, сделанные через оверлей Aurora, появятся здесь.">
      {#snippet icon()}
        <Image size={22} strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button onclick={() => toast('Папка скриншотов недоступна в demo')}>Открыть папку скриншотов</Button>
      {/snippet}
    </EmptyState>
  {:else if tab === 'notes'}
    <EmptyState title="Заметок пока нет" description="Храните здесь пароли от сейфов, билды и прочие игровые записи.">
      {#snippet icon()}
        <NotebookPen size={22} strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button onclick={() => toast('Заметки недоступны в demo')}>
          <SquarePen size={15} strokeWidth={1.8} />
          Добавить заметку
        </Button>
      {/snippet}
    </EmptyState>
  {/if}
{/if}

<style>
  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: var(--space-3) 0 var(--space-4);
    color: var(--text-3);
  }

  .crumb {
    font-size: 13.5px;
    color: var(--text-3);
    border-radius: 6px;
    transition: color var(--dur) var(--ease);
  }

  button.crumb:hover {
    color: var(--text);
  }

  .crumb.current {
    color: var(--text);
    font-weight: 500;
  }

  .title {
    font-size: clamp(32px, 3.4vw, 44px);
    letter-spacing: -0.015em;
    text-shadow: 0 2px 18px rgba(0, 0, 0, 0.5);
  }

  .genres {
    display: flex;
    gap: 8px;
    margin-top: 14px;
    flex-wrap: wrap;
  }

  .genre {
    padding: 4px 12px;
    border-radius: 8px;
    background: rgba(10, 14, 19, 0.55);
    border: 1px solid rgba(255, 255, 255, 0.1);
    font-size: 12.5px;
    color: var(--text-2);
    backdrop-filter: blur(4px);
  }

  .meta {
    display: flex;
    gap: var(--space-6);
    margin-top: var(--space-4);
    flex-wrap: wrap;
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-size: 13px;
    color: rgba(243, 245, 247, 0.9);
    text-shadow: 0 1px 6px rgba(0, 0, 0, 0.5);
  }

  .meta-item :global(svg) {
    color: var(--text-2);
  }

  .meta-label {
    color: var(--text-2);
  }

  .tagline {
    margin-top: var(--space-4);
    max-width: 460px;
    font-size: 14px;
    color: rgba(243, 245, 247, 0.85);
    text-shadow: 0 1px 8px rgba(0, 0, 0, 0.5);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-5);
  }

  .installed-group {
    display: inline-flex;
  }

  .installed-group :global(button:first-child) {
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
    border-right: none;
  }

  .installed-group :global(button:last-child) {
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
    padding: 0 10px;
  }

  .size-hint {
    font-size: 13.5px;
    color: rgba(243, 245, 247, 0.75);
    text-shadow: 0 1px 6px rgba(0, 0, 0, 0.5);
  }

  .tabs-row {
    margin: var(--space-6) 0;
  }

  .overview {
    display: grid;
    grid-template-columns: 1.15fr 1.2fr 1fr;
    gap: var(--space-5);
    align-items: start;
    margin-bottom: var(--space-5);
  }

  .card-title {
    font-size: 15px;
    font-weight: 600;
  }

  .card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 2px;
  }

  .description {
    margin-top: var(--space-3);
    font-size: 13.5px;
    line-height: 1.6;
    color: var(--text-2);
  }

  .divider {
    height: 1px;
    background: var(--border);
    margin: var(--space-4) 0;
  }

  .props {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .prop {
    display: flex;
    justify-content: space-between;
    gap: var(--space-4);
  }

  dt {
    font-size: 13px;
    color: var(--text-3);
  }

  dd {
    font-size: 13px;
    color: var(--text-2);
    text-align: right;
  }

  .expand {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    margin-top: var(--space-4);
    font-size: 13.5px;
    font-weight: 500;
    color: var(--accent-text);
    border-radius: 6px;
    transition: color var(--dur) var(--ease);
  }

  .expand:hover {
    color: var(--accent-hover);
  }

  .expand :global(svg) {
    transition: transform var(--dur) var(--ease);
  }

  .link {
    font-size: 13px;
    color: var(--accent-text);
    border-radius: 6px;
    transition: color var(--dur) var(--ease);
  }

  .link:hover {
    color: var(--accent-hover);
  }

  .link.with-icon {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-top: var(--space-4);
  }

  .ach-summary {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin: var(--space-4) 0;
  }

  .ach-badge {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 52px;
    height: 52px;
    border-radius: 50%;
    background: var(--accent-subtle);
    color: var(--accent-text);
    flex-shrink: 0;
  }

  .ach-count {
    display: flex;
    flex-direction: column;
  }

  .ach-nums {
    font-size: 15px;
    color: var(--text-3);
  }

  .ach-nums strong {
    font-size: 26px;
    font-weight: 600;
    color: var(--text);
  }

  .ach-label {
    font-size: 13px;
    color: var(--text-3);
  }

  .ach-recent {
    margin-top: var(--space-4);
  }

  .ach-recent-label {
    display: block;
    font-size: 12px;
    color: var(--text-3);
    margin-bottom: 10px;
  }

  .ach-list {
    display: flex;
    flex-direction: column;
    margin-top: var(--space-5);
  }

  .ach-list .ach-item + .ach-item {
    border-top: 1px solid var(--border);
  }

  .ach-item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 9px 0;
  }

  .ach-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    background: rgba(232, 195, 90, 0.12);
    color: #e8c35a;
    flex-shrink: 0;
  }

  .ach-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .ach-name {
    font-size: 13.5px;
    font-weight: 550;
  }

  .ach-desc {
    font-size: 12.5px;
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .ach-date {
    font-size: 12px;
    color: var(--text-3);
    white-space: nowrap;
  }

  .muted {
    font-size: 13.5px;
    color: var(--text-3);
    margin-top: var(--space-3);
  }

  .muted.small {
    font-size: 12.5px;
    margin: 0;
  }

  .dlc-list {
    display: flex;
    flex-direction: column;
    margin-top: var(--space-2);
  }

  .dlc {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: 10px 0;
  }

  .dlc + .dlc {
    border-top: 1px solid var(--border);
  }

  .dlc-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .dlc-name {
    font-size: 13.5px;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dlc-kind {
    font-size: 12px;
    color: var(--text-3);
  }

  .dlc-state {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12.5px;
    color: var(--text-3);
    white-space: nowrap;
  }

  .dlc-state.installed :global(svg) {
    color: var(--success);
  }

  .addons {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    max-width: 720px;
  }

  .addon {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .qa-title {
    margin-bottom: var(--space-4);
  }

  .quick-actions {
    display: grid;
    grid-template-columns: repeat(6, 1fr);
    gap: var(--space-2);
  }

  .qa {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 14px;
    border-radius: var(--radius-md);
    text-align: left;
    color: var(--text-2);
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .qa:hover {
    background: rgba(255, 255, 255, 0.04);
    color: var(--text);
  }

  .qa :global(svg) {
    flex-shrink: 0;
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  .qa:hover :global(svg) {
    color: var(--text-2);
  }

  .qa.danger:hover {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .qa.danger:hover :global(svg) {
    color: var(--danger);
  }

  .qa-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .qa-label {
    font-size: 13.5px;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .qa-sub {
    font-size: 11.5px;
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  @media (max-width: 1400px) {
    .overview {
      grid-template-columns: 1fr 1fr;
    }

    .quick-actions {
      grid-template-columns: repeat(3, 1fr);
    }
  }

  @media (max-width: 1100px) {
    .overview {
      grid-template-columns: 1fr;
    }
  }
</style>
