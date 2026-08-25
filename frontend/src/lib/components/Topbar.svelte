<script lang="ts">
  import {
    Bell,
    ChevronLeft,
    ChevronRight,
    Minus,
    Square,
    X,
  } from '@lucide/svelte';
  import { Window } from '@wailsio/runtime';
  import { onDestroy } from 'svelte';
  import { SearchOverlay, initialState } from '../search/overlay';
  import { inWails } from '../services/backend';
  import { searchAll, type GameHit, type ReleaseHit } from '../services/search';
  import { canGoBack, canGoForward, goBack, goForward, navigate } from '../stores/router';
  import { notifications, type Notification } from '../stores/notifications';
  import { toast } from '../stores/toasts';
  import { clickOutside } from '../utils/clickOutside';
  import { bytesSize, plural } from '../utils/format';
  import Artwork from './Artwork.svelte';
  import IconButton from './IconButton.svelte';
  import SearchInput from './SearchInput.svelte';

  let search = $state('');
  let searchInput = $state<HTMLInputElement | undefined>(undefined);
  let results = $state(initialState());
  let resultsBox = $state<HTMLElement | undefined>(undefined);
  let notificationsOpen = $state(false);

  const overlay = new SearchOverlay({
    search: searchAll,
    onState: (state) => (results = state),
  });

  onDestroy(() => overlay.destroy());

  const total = $derived(results.games.length + results.releases.length);
  const showEmpty = $derived(
    results.searched && !results.loading && !results.error && total === 0,
  );

  $effect(() => {
    const index = results.active;
    if (index < 0 || !resultsBox) return;
    resultsBox.querySelectorAll('.result')[index]?.scrollIntoView({ block: 'nearest' });
  });

  function openGame(hit: GameHit) {
    overlay.close();
    navigate('game', { id: hit.id });
  }

  function openRelease(hit: ReleaseHit) {
    overlay.close();
    navigate('sources', { sourceId: hit.sourceId, releaseId: hit.id });
  }

  function openActive() {
    const active = overlay.activeHit();
    if (!active) return;
    if (active.kind === 'game') openGame(active.hit);
    else openRelease(active.hit);
  }

  function onSearchKeydown(event: KeyboardEvent) {
    switch (event.key) {
      case 'Escape':
        if (results.open) overlay.close();
        else searchInput?.blur();
        return;
      case 'ArrowDown':
        event.preventDefault();
        overlay.move(1);
        return;
      case 'ArrowUp':
        event.preventDefault();
        overlay.move(-1);
        return;
      case 'Enter':
        if (results.active < 0) return;
        event.preventDefault();
        openActive();
    }
  }

  function onWindowKeydown(event: KeyboardEvent) {
    if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'k') return;
    event.preventDefault();
    searchInput?.focus();
    overlay.focus();
  }

  function gameHint(hit: GameHit) {
    const parts: string[] = [];
    if (hit.year) parts.push(String(hit.year));
    const version = hit.latestVersion || hit.version;
    if (version) parts.push(version);
    if (hit.releases > 0) {
      parts.push(`${hit.releases} ${plural(hit.releases, 'релиз', 'релиза', 'релизов')}`);
    }
    if (hit.sources > 1) {
      parts.push(`${hit.sources} ${plural(hit.sources, 'источник', 'источника', 'источников')}`);
    }
    return parts.join(' · ');
  }

  function openNotification(item: Notification) {
    notificationsOpen = false;
    navigate(item.route);
  }

  // Окно без рамки, поэтому поведение нативного заголовка приходится повторять вручную.
  function onTitlebarDoubleClick(event: MouseEvent) {
    const target = event.target as HTMLElement | null;
    if (target?.closest('.no-drag')) return;
    win('maximise');
  }

  function win(action: 'minimise' | 'maximise' | 'close') {
    if (!inWails) {
      toast('Доступно только в desktop-сборке');
      return;
    }
    if (action === 'minimise') Window.Minimise();
    else if (action === 'maximise') Window.ToggleMaximise();
    else Window.Close();
  }
</script>

<svelte:window onkeydown={onWindowKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<header class="topbar" style="--wails-draggable: drag" ondblclick={onTitlebarDoubleClick}>
  <div class="no-drag nav-buttons">
    <IconButton label="Назад" disabled={!$canGoBack} onclick={goBack}>
      <ChevronLeft size="1.8rem" strokeWidth={1.8} />
    </IconButton>
    <IconButton label="Вперёд" disabled={!$canGoForward} onclick={goForward}>
      <ChevronRight size="1.8rem" strokeWidth={1.8} />
    </IconButton>
  </div>

  <div class="no-drag search-wrap" use:clickOutside={() => overlay.close()}>
    <SearchInput
      bind:value={search}
      bind:input={searchInput}
      placeholder="Поиск игр и релизов"
      shortcut="Ctrl K"
      loading={results.loading}
      oninput={(value) => overlay.setQuery(value)}
      onfocus={() => overlay.focus()}
      onkeydown={onSearchKeydown}
    />
    {#if results.open && results.query.trim() !== ''}
      <div class="results" bind:this={resultsBox}>
        {#if results.games.length > 0}
          <div class="results-head">Игры</div>
          {#each results.games as hit, i (hit.id)}
            <button class="result" class:active={results.active === i} onclick={() => openGame(hit)}>
              <span class="result-art">
                <Artwork src={hit.cover ?? ''} alt={hit.title} ratio="3 / 4" radius="var(--radius-xs)" />
              </span>
              <span class="result-body">
                <span class="result-line">
                  <span class="result-title">{hit.title}</span>
                  {#if hit.installed}
                    <span class="result-badge">Установлено</span>
                  {/if}
                </span>
                {#if gameHint(hit)}
                  <span class="result-hint">{gameHint(hit)}</span>
                {/if}
              </span>
            </button>
          {/each}
          {#if results.moreGames > 0}
            <div class="results-more">и ещё {results.moreGames}</div>
          {/if}
        {/if}

        {#if results.releases.length > 0}
          <div class="results-head">Релизы без совпадения</div>
          {#each results.releases as hit, i (hit.id)}
            <button
              class="result"
              class:active={results.active === results.games.length + i}
              onclick={() => openRelease(hit)}
            >
              <span class="result-body">
                <span class="result-title">{hit.title}</span>
                <span class="result-hint">
                  {hit.sourceName}{hit.size > 0 ? ` · ${bytesSize(hit.size)}` : ''}
                </span>
              </span>
            </button>
          {/each}
          {#if results.moreReleases > 0}
            <div class="results-more">и ещё {results.moreReleases}</div>
          {/if}
        {/if}

        {#if results.error}
          <div class="results-error">{results.error}</div>
        {:else if showEmpty}
          <div class="results-empty">Ничего не найдено</div>
        {/if}
      </div>
    {/if}
  </div>

  <div class="no-drag right">
    <div class="bell" use:clickOutside={() => (notificationsOpen = false)}>
      <IconButton label="Уведомления" active={notificationsOpen} onclick={() => (notificationsOpen = !notificationsOpen)}>
        <Bell size="1.8rem" strokeWidth={1.8} />
      </IconButton>
      {#if $notifications.length > 0}
        <span class="badge">{$notifications.length}</span>
      {/if}
      {#if notificationsOpen}
        <div class="notifications">
          <div class="notifications-head">Уведомления</div>
          {#if $notifications.length === 0}
            <div class="notifications-empty">Пока ничего нового</div>
          {:else}
            {#each $notifications as n (n.id)}
              <button class="notification" onclick={() => openNotification(n)}>
                <span class="notification-title">{n.title}</span>
                <span class="notification-text">{n.text}</span>
              </button>
            {/each}
          {/if}
        </div>
      {/if}
    </div>

    <div class="window-controls">
      <button class="wc" aria-label="Свернуть" onclick={() => win('minimise')}>
        <Minus size="1.6rem" strokeWidth={1.6} />
      </button>
      <button class="wc" aria-label="Развернуть" onclick={() => win('maximise')}>
        <Square size="1.2rem" strokeWidth={1.6} />
      </button>
      <button class="wc close" aria-label="Закрыть" onclick={() => win('close')}>
        <X size="1.6rem" strokeWidth={1.6} />
      </button>
    </div>
  </div>
</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    height: var(--topbar-h);
    padding: 0 var(--space-3) 0 var(--page-x);
    flex-shrink: 0;
  }

  .no-drag {
    --wails-draggable: no-drag;
  }

  .nav-buttons {
    display: flex;
    gap: 0;
    margin-left: -0.8rem;
  }

  .search-wrap {
    position: relative;
    flex: 1;
    max-width: 44rem;
  }

  .results {
    position: absolute;
    z-index: 50;
    top: calc(100% + 0.6rem);
    left: 0;
    right: 0;
    max-height: 46rem;
    overflow-y: auto;
    padding: 0.5rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
  }

  .results-head,
  .notifications-head {
    padding: 0.8rem 1rem 0.5rem;
    font-size: 1.1rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
  }

  .result {
    display: flex;
    align-items: center;
    gap: 1rem;
    width: 100%;
    padding: 0.6rem 1rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .notification {
    display: flex;
    flex-direction: column;
    gap: 1px;
    width: 100%;
    padding: 0.8rem 1rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .result-art {
    width: 3rem;
    flex-shrink: 0;
  }

  .result-body {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    flex: 1;
  }

  .result-line {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
  }

  .result-badge {
    flex-shrink: 0;
    padding: 1px 0.6rem;
    border-radius: var(--radius-xs);
    background: var(--hover-strong);
    font-size: 1.1rem;
    color: var(--text-3);
  }

  .result:hover,
  .result.active,
  .notification:hover {
    background: var(--hover-strong);
  }

  .result-title,
  .notification-title {
    font-size: var(--font-sm);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .result-hint,
  .notification-text {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .notifications-empty {
    padding: 0.8rem 1rem 1.2rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .badge {
    position: absolute;
    top: 0;
    right: 0;
    pointer-events: none;
    min-width: 1.6rem;
    height: 1.6rem;
    padding: 0 0.4rem;
    border-radius: 0.8rem;
    background: var(--accent);
    color: #fff;
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.6rem;
    text-align: center;
  }

  .results-more,
  .results-empty,
  .results-error {
    padding: 0.8rem 1rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .results-error {
    color: var(--danger);
  }

  .right {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-left: auto;
  }

  .bell {
    position: relative;
  }

  .notifications {
    position: absolute;
    z-index: 50;
    top: calc(100% + 0.6rem);
    right: 0;
    width: 32rem;
    padding: 0.5rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    animation: pop var(--dur-fast) var(--ease);
  }

  .window-controls {
    display: flex;
    margin-left: 1.2rem;
  }

  .wc {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4.2rem;
    height: 3.2rem;
    border-radius: var(--radius-sm);
    color: var(--text-3);
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .wc:hover {
    background: var(--hover-strong);
    color: var(--text);
  }

  .wc.close:hover {
    background: #c9403f;
    color: #fff;
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(-0.3rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
