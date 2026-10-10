<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { RotateCcw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProfileCanvas from '../../lib/components/ProfileCanvas.svelte';
  import ProfileCover from '../../lib/components/ProfileCover.svelte';
  import AboutBlock from '../../lib/components/profile/AboutBlock.svelte';
  import BlockGrid from '../../lib/components/profile/BlockGrid.svelte';
  import BlockPicker from '../../lib/components/profile/BlockPicker.svelte';
  import BlockSettings from '../../lib/components/profile/BlockSettings.svelte';
  import type { ConfirmPrompt } from '../../lib/confirm/prompts';
  import { msg } from '../../lib/i18n';
  import { appearanceOf } from '../../lib/profile/appearance';
  import {
    addBlock,
    defaultLayout,
    blockIssue,
    effectiveLayout,
    layoutErrorText,
    layoutPatch,
    moveBlock,
    removeBlock,
    sameLayout,
    setWidth,
    statusIssue,
    updateConfig,
    validateLayout,
    visibleBlocks,
    type AddPreset,
    type LayoutResult,
    type AddResult,
  } from '../../lib/profile/layout';
  import { ownAutoArt, resolveOwn, type GridBlock } from '../../lib/profile/layoutView';
  import { coverOf } from '../../lib/profile/view';
  import { type ProfileSettings, type BlockWidth, DEFAULT_PROFILE } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { getProfilePreview, type GameRef, type ProfileSnapshot } from '../../lib/services/profile';
  import { libraryGames } from '../../lib/stores/library';
  import { gameArt, loadArt } from '../../lib/stores/metadata';
  import { initProfile, profileDraft, profileFailed, profileSnapshot } from '../../lib/stores/profile';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { authState, currentUser, isOffline, saveProfile } from '../../lib/stores/user';
  import { playtime, relativeDate } from '../../lib/utils/format';
  import HiddenBadge from './HiddenBadge.svelte';
  import ProfileActivity from './ProfileActivity.svelte';
  import ProfileAppearancePanel from './ProfileAppearancePanel.svelte';
  import ProfileHeader from './ProfileHeader.svelte';
  import ProfilePlaying from './ProfilePlaying.svelte';
  import ProfileSettingsModal from './ProfileSettingsModal.svelte';
  import ProfileStats from './ProfileStats.svelte';

  function copyOf(source: ProfileSettings): ProfileSettings {
    const copy = JSON.parse(JSON.stringify(source)) as ProfileSettings;
    return { ...copy, appearance: appearanceOf(copy.appearance), layout: effectiveLayout(copy) };
  }

  function withoutLayout(source: ProfileSettings): string {
    const { layout: _layout, ...rest } = source;
    return JSON.stringify(rest);
  }

  let editing = $state(false);
  let draft = $state<ProfileSettings>(DEFAULT_PROFILE);
  let origin = $state.raw<ProfileSettings>(DEFAULT_PROFILE);
  let originReset = false;
  let owner = '';
  let resetLayout = $state(false);
  let saving = $state(false);
  let saveError = $state('');
  let layoutError = $state('');
  let selectedId = $state('');
  let picked = $state<Record<number, GameRef>>({});
  let full = $state<ProfileSnapshot | null>(null);
  let previewFailed = $state(false);
  let settingsOpen = $state(false);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => void } | null>(null);

  const isGuest = $derived($authState === 'guest');
  const settings = $derived((isGuest ? null : $currentUser?.profile) ?? DEFAULT_PROFILE);
  const shown = $derived(editing ? draft : settings);
  const appearance = $derived(appearanceOf(shown.appearance));
  const layout = $derived(shown.layout ?? effectiveLayout(shown));
  const playingNow = $derived($profileSnapshot.running[0] ?? null);
  const bio = $derived(!isGuest ? ($currentUser?.bio ?? '') : '');
  const selected = $derived(layout.blocks.find((block) => block.id === selectedId) ?? null);
  const issues = $derived(validateLayout(layout));
  const hiddenTypes = $derived(
    new Set(layout.blocks.filter((block) => !visibleBlocks(layout, shown).includes(block)).map((block) => block.type)),
  );
  const wantsFull = $derived(editing || (appearance.theme === 'auto' && appearance.autoSource === 'most_played'));
  const dirty = $derived(
    editing &&
      (resetLayout !== originReset ||
        !sameLayout(draft.layout, origin.layout) ||
        withoutLayout($state.snapshot(draft) as ProfileSettings) !== withoutLayout(origin)),
  );
  const statusProblem = $derived(editing ? statusIssue(draft.statusText) : null);
  const canSave = $derived(dirty && issues.length === 0 && statusProblem === null && !saving && !$isOffline);

  function empty(type: string): boolean {
    const snapshot = $profileSnapshot;
    if (type === 'playing') return !playingNow;
    if (type === 'recent') return snapshot.playing.length === 0;
    if (type === 'activity') return snapshot.activity.every((day) => day.entries.length === 0);
    if (type === 'about') return bio.trim() === '';
    return false;
  }

  const gridBlocks = $derived(
    resolveOwn(layout, {
      flags: shown,
      snapshot: $profileSnapshot,
      preview: editing ? full : null,
      art: $gameArt,
      picked,
      open: (game) => navigate('game', { id: game.id }),
      empty,
    }),
  );

  const autoArt = $derived(
    appearance.theme === 'auto'
      ? ownAutoArt(appearance.autoSource, {
          running: $profileSnapshot.running,
          mostPlayed: (full ?? $profileSnapshot).showcase.find((block) => block.kind === 'most_played')?.games[0],
          blocks: gridBlocks,
          art: $gameArt,
        }) || undefined
      : undefined,
  );

  function lastPlayedOf(id: string): string | null {
    return $libraryGames.find((game) => game.id === id)?.lastPlayed ?? null;
  }

  function titleOf(igdbId: number): string {
    return (
      picked[igdbId]?.title ??
      $profileSnapshot.layoutGames?.[String(igdbId)]?.game?.title ??
      msg('profile.gameUnavailable')
    );
  }

  function startEditing() {
    if (editing || isGuest) return;
    if ($isOffline) {
      toast(msg('social.editRequiresConnection'));
      return;
    }
    owner = $currentUser?.id ?? '';
    origin = copyOf(settings);
    originReset = false;
    const stash = get(profileDraft);
    if (stash && stash.owner === owner) {
      draft = stash.draft;
      resetLayout = stash.reset;
      profileDraft.set(null);
      toast(msg('profile.draftRestored'));
    } else {
      draft = copyOf(settings);
      resetLayout = false;
    }
    saveError = '';
    layoutError = '';
    selectedId = '';
    settingsOpen = false;
    editing = true;
  }

  function leaveEditing() {
    editing = false;
    selectedId = '';
    saveError = '';
    layoutError = '';
    resetLayout = false;
    profileDraft.set(null);
  }

  function cancel() {
    if (!dirty) {
      leaveEditing();
      return;
    }
    pending = {
      prompt: {
        title: msg('profile.discardTitle'),
        text: msg('profile.discardText'),
        confirm: msg('profile.discardConfirm'),
        cancel: msg('profile.keepEditing'),
      },
      run: leaveEditing,
    };
  }

  function askReset() {
    pending = {
      prompt: {
        title: msg('profile.resetLayoutTitle'),
        text: msg('profile.resetLayoutText'),
        confirm: msg('profile.resetLayout'),
      },
      run: () => {
        draft.layout = defaultLayout(draft);
        resetLayout = true;
        selectedId = '';
        layoutError = '';
      },
    };
  }

  function openSettings() {
    if (editing) {
      toast(msg('profile.finishEditingFirst'));
      return;
    }
    settingsOpen = true;
  }

  function apply(result: LayoutResult | AddResult): boolean {
    if (!result.ok) {
      layoutError = layoutErrorText(result.error);
      return false;
    }
    layoutError = '';
    resetLayout = false;
    draft.layout = result.layout;
    return true;
  }

  function add(preset: AddPreset) {
    const result = addBlock(layout, preset.type, { width: preset.width, config: preset.config });
    if (apply(result) && result.ok) selectedId = result.id;
  }

  function remove(id: string) {
    if (apply(removeBlock(layout, id)) && selectedId === id) selectedId = '';
  }

  function move(from: number, to: number) {
    apply(moveBlock(layout, from, to));
  }

  function resize(id: string, width: BlockWidth) {
    apply(setWidth(layout, id, width));
  }

  function configure(id: string, patch: Record<string, unknown>) {
    apply(updateConfig(layout, id, patch));
  }

  function remember(pick: { igdbId: number; game: GameRef }) {
    picked = { ...picked, [pick.igdbId]: pick.game };
  }

  async function save() {
    if (!canSave) return;
    saving = true;
    saveError = '';
    try {
      const profile = $state.snapshot(draft) as ProfileSettings;
      const patch = layoutPatch(settings, layout, resetLayout);
      if (patch.send) profile.layout = patch.layout;
      else delete profile.layout;
      const saved = await saveProfile({ profile });
      if (!saved) {
        saveError = msg('profile.saveBusy');
        return;
      }
      if ($currentUser?.id !== owner) return;
      toast(msg('social.settingsSaved'), 'success');
      leaveEditing();
    } catch (err) {
      saveError = accountErrorText(err, msg('profile.saveFailed'));
    } finally {
      saving = false;
    }
  }

  $effect(() => {
    const snapshot = $profileSnapshot;
    const ids = [
      ...snapshot.playing.map((entry) => entry.game.canonicalGameId),
      ...snapshot.running.map((game) => game.canonicalGameId),
      ...(full ?? snapshot).showcase.flatMap((block) => block.games.map((game) => game.canonicalGameId)),
      ...Object.values(snapshot.layoutGames ?? {}).map((entry) => entry.game?.canonicalGameId),
      ...Object.values(picked).map((game) => game.canonicalGameId),
    ].filter((id): id is string => Boolean(id));
    if (ids.length > 0) loadArt(ids);
  });

  $effect(() => {
    if (editing && $currentUser?.id !== owner) leaveEditing();
  });

  $effect(() => {
    let active = true;
    $currentUser?.id;
    previewFailed = false;
    if (wantsFull) {
      getProfilePreview()
        .then((snapshot) => { if (active) full = snapshot; })
        .catch(() => { if (active) previewFailed = true; });
    } else { full = null; }
    return () => { active = false; };
  });

  onMount(() => {
    initProfile();
    const stash = get(profileDraft);
    if (stash && stash.owner === get(currentUser)?.id) startEditing();
  });

  onDestroy(() => {
    if (editing && dirty) {
      profileDraft.set({ owner, draft: $state.snapshot(draft) as ProfileSettings, reset: resetLayout });
    }
  });
</script>

<PageHeader title={msg('social.profileLabel')} />

{#if $profileFailed}
  <p class="load-error" role="alert">{msg('profile.loadFailed')}</p>
{/if}
{#if previewFailed}
  <p class="load-error" role="alert">{msg('profile.previewLoadFailed')}</p>
{/if}

{#snippet flag()}
  <HiddenBadge text={msg('social.hiddenGenericHint')} />
{/snippet}

{#snippet external(block: GridBlock)}
  {#if block.type === 'playing' && playingNow}
    <ProfilePlaying
      title={playingNow.title}
      art={coverOf(playingNow, $gameArt)}
      hidden={hiddenTypes.has('playing')}
      disabled={playingNow.archived}
      onopen={() => navigate('game', { id: playingNow.id })}
    />
  {:else if block.type === 'recent'}
    <Card title={msg('social.recentlyPlayedTitle')}>
      <div class="recent-row">
        {#each $profileSnapshot.playing as entry (entry.game.id)}
          <div class="recent-item">
            <GameCard id={entry.game.id} title={entry.game.title} cover={coverOf(entry.game, $gameArt)} variant="capsule">
              {#snippet footer()}
                <span class="recent-meta">
                  <span class="dot"></span>
                  {playtime(entry.game.playtimeSeconds)} · {relativeDate(lastPlayedOf(entry.game.id))}
                </span>
              {/snippet}
            </GameCard>
          </div>
        {/each}
      </div>
    </Card>
  {:else if block.type === 'activity'}
    <ProfileActivity days={$profileSnapshot.activity} hidden={hiddenTypes.has('activity')} />
  {:else if block.type === 'stats'}
    <Card title={msg('profile.blockStats')}>
      <ProfileStats stats={$profileSnapshot.stats} hidden={hiddenTypes.has('stats')} />
    </Card>
  {:else if block.type === 'about'}
    <AboutBlock {bio} />
  {/if}
{/snippet}

<div class="workspace" class:editing>
  <div class="profile">
    {#if editing}
      <div class="topbar">
        <div class="topbar-text">
          <strong>{msg('profile.editingTitle')}</strong>
          {#if saveError || layoutError}
            <span class="topbar-sub error" role="alert">{saveError || layoutError}</span>
          {:else if statusProblem}
            <span class="topbar-sub error" role="alert">{layoutErrorText(statusProblem)}</span>
          {:else if issues.length > 0}
            <span class="topbar-sub">{msg('profile.fillBlocks')}</span>
          {:else}
            <span class="topbar-sub">{msg('profile.editingHint')}</span>
          {/if}
        </div>
        <Button size="sm" variant="ghost" disabled={saving} onclick={askReset}>
          <RotateCcw size="1.4rem" strokeWidth={1.8} />{msg('profile.resetLayout')}
        </Button>
        <Button variant="ghost" disabled={saving} onclick={cancel}>{msg('common.cancel')}</Button>
        <Button variant="primary" disabled={!canSave} onclick={save}>
          {saving ? msg('social.saving') : msg('common.save')}
        </Button>
      </div>
    {/if}
    <ProfileCanvas {appearance} {autoArt}>
      <ProfileCover {appearance} {autoArt} />
      <ProfileHeader
        running={$profileSnapshot.running}
        stats={$profileSnapshot.stats}
        showOnline={shown.showOnline}
        showPlaying={shown.showPlaying}
        showStats={shown.showStats}
        {appearance}
        statusEmoji={shown.statusEmoji ?? ''}
        statusText={shown.statusText ?? ''}
        onedit={startEditing}
        onsettings={openSettings}
      />

      {#if !isGuest}
        <div class="blocks">
          <BlockGrid
            blocks={gridBlocks}
            {external}
            {flag}
            accent={appearance.accent}
            {editing}
            {selectedId}
            onselect={(id) => (selectedId = id)}
            onmove={move}
            onwidth={resize}
            onremove={remove}
          />
          {#if editing}
            <BlockPicker {layout} disabled={saving} onadd={add} />
          {/if}
        </div>
      {/if}
    </ProfileCanvas>
  </div>
  {#if !isGuest && editing}
    <div class="side">
      {#if selected}
        {#key selected.id}
          <BlockSettings
            block={selected}
            {layout}
            {titleOf}
            issue={blockIssue(selected)}
            disabled={saving}
            onconfig={(patch) => configure(selected.id, patch)}
            onremember={remember}
            onremove={() => remove(selected.id)}
            onclose={() => (selectedId = '')}
          />
        {/key}
      {:else}
        {#key $currentUser?.id}
          <ProfileAppearancePanel bind:draft disabled={saving} />
        {/key}
      {/if}
    </div>
  {/if}
</div>

{#if !isGuest && settingsOpen}
  <ProfileSettingsModal bind:open={settingsOpen} {settings} />
{/if}

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}

<style>
  .load-error {
    margin: 0 0 var(--space-4);
    font-size: var(--font-sm);
    color: var(--danger);
  }

  .workspace {
    display: flex;
    gap: 1.6rem;
    align-items: flex-start;
  }

  .profile {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .side {
    width: 30rem;
    flex-shrink: 0;
    position: sticky;
    top: 1.6rem;
    align-self: flex-start;
  }

  .topbar {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
    padding: var(--space-3) var(--space-4);
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-pop);
  }

  .topbar-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .topbar-text strong {
    font-size: var(--font-md);
    font-weight: 600;
  }

  .topbar-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow-wrap: anywhere;
  }

  .topbar-sub.error {
    color: var(--danger);
  }

  .blocks {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
  }

  .recent-row {
    display: flex;
    gap: var(--space-4);
    overflow-x: auto;
    margin: calc(-1 * var(--space-2)) calc(-1 * var(--space-2)) 0;
    padding: var(--space-2) var(--space-2) var(--space-3);
  }

  .recent-item {
    flex: 0 0 auto;
    width: 22rem;
  }

  .recent-meta {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .recent-meta .dot {
    width: 0.7rem;
    height: 0.7rem;
    border-radius: 50%;
    background: var(--success);
    flex-shrink: 0;
  }

  @media (max-width: 1000px) {
    .workspace.editing {
      flex-direction: column-reverse;
    }

    .workspace.editing .side {
      position: static;
      width: 100%;
    }

    .profile {
      width: 100%;
    }
  }
</style>
