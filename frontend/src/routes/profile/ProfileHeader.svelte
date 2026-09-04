<script lang="ts">
  import { Calendar, Copy, EllipsisVertical, Gamepad2, LogIn } from '@lucide/svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import AvatarEditor from '../../lib/components/AvatarEditor.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import MaskedEmail from '../../lib/components/MaskedEmail.svelte';
  import { accountErrorField, accountErrorText } from '../../lib/services/accountMessages';
  import type { GameRef, ProfileStats as ProfileStatsData } from '../../lib/services/profile';
  import { friendCode } from '../../lib/services/social';
  import { joinDate } from '../../lib/social/view';
  import { statusDot } from '../../lib/social/presence';
  import { presenceStatus } from '../../lib/stores/presence';
  import { settings } from '../../lib/stores/settings';
  import { authState, currentUser, isOffline, leaveGuest, saveProfile, savingProfile, signOut } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';
  import HiddenBadge from './HiddenBadge.svelte';
  import ProfileStats from './ProfileStats.svelte';

  type MenuItem = { id: string; label: string; danger?: boolean; separator?: boolean };

  let {
    running,
    stats,
    showOnline,
    showPlaying,
    showStats,
    onsettings,
  }: {
    running: GameRef[];
    stats: ProfileStatsData;
    showOnline: boolean;
    showPlaying: boolean;
    showStats: boolean;
    onsettings: () => void;
  } = $props();

  const BIO_LIMIT = 150;

  let editing = $state(false);
  let draft = $state({ displayName: '', username: '', bio: '' });
  let fieldErrors = $state<{ displayName?: string; username?: string; bio?: string; general?: string }>({});
  let busy = $state(false);
  let code = $state('');
  let warnedCode = false;

  const isGuest = $derived($authState === 'guest');

  const avatarName = $derived(
    !isGuest && $currentUser ? $currentUser.displayName || $currentUser.username : 'Гость',
  );

  const memberSince = $derived($currentUser ? joinDate($currentUser.createdAt) : '');
  const playing = $derived(running[0] ?? null);
  const playingHidden = $derived(!!playing && !showPlaying);

  const dirty = $derived(
    !!$currentUser &&
      (draft.displayName !== $currentUser.displayName ||
        draft.username !== $currentUser.username ||
        draft.bio !== $currentUser.bio),
  );

  const bio = $derived(!isGuest ? ($currentUser?.bio ?? '') : '');

  $effect(() => {
    if (isGuest || $authState !== 'authenticated' || !$settings?.accountSync) {
      code = '';
      return;
    }
    let cancelled = false;
    friendCode()
      .then((value) => {
        if (!cancelled) code = value;
      })
      .catch((err) => {
        if (cancelled) return;
        code = '';
        if (warnedCode) return;
        warnedCode = true;
        console.warn('friend code request failed', err);
      });
    return () => {
      cancelled = true;
    };
  });

  async function copyCode() {
    try {
      await navigator.clipboard.writeText(code);
      toast('Скопировано', 'info');
    } catch {
      toast('Не удалось скопировать', 'danger');
    }
  }

  const menuItems = $derived<MenuItem[]>([
    { id: 'edit', label: 'Редактировать' },
    { id: 'settings', label: 'Настройки профиля' },
    { id: 'signout', label: busy ? 'Выход…' : 'Выйти', danger: true, separator: true },
  ]);

  function startEditing() {
    if (!$currentUser || $isOffline) return;
    draft = { displayName: $currentUser.displayName, username: $currentUser.username, bio: $currentUser.bio };
    fieldErrors = {};
    editing = true;
  }

  function cancelEditing() {
    fieldErrors = {};
    editing = false;
  }

  async function save() {
    if (!$currentUser || !dirty || $savingProfile || $isOffline) return;
    fieldErrors = {};
    const patch: { displayName?: string; username?: string; bio?: string } = {};
    if (draft.displayName !== $currentUser.displayName) patch.displayName = draft.displayName;
    if (draft.username !== $currentUser.username) patch.username = draft.username;
    if (draft.bio !== $currentUser.bio) patch.bio = draft.bio;
    try {
      await saveProfile(patch);
      editing = false;
      toast('Профиль обновлён', 'success');
    } catch (err) {
      const message = accountErrorText(err, 'Не удалось сохранить');
      const field = accountErrorField(err);
      if (field === 'username') fieldErrors = { username: message };
      else if (field === 'displayName') fieldErrors = { displayName: message };
      else if (field === 'bio') fieldErrors = { bio: message };
      else fieldErrors = { general: message };
    }
  }

  async function onMenu(id: string) {
    if (id === 'edit') {
      if ($isOffline) toast('Изменить профиль можно только при связи с сервером');
      else startEditing();
    } else if (id === 'settings') {
      onsettings();
    } else if (id === 'signout') {
      await run(signOut, 'Не удалось выйти');
    }
  }

  async function run(fn: () => Promise<unknown>, fallback: string) {
    if (busy) return;
    busy = true;
    try {
      await fn();
    } catch (err) {
      toast(accountErrorText(err, fallback), 'danger');
    } finally {
      busy = false;
    }
  }
</script>

<section class="profile-header">
  <Card>
    <div class="head">
      <Avatar
        size="lg"
        name={avatarName}
        src={isGuest ? undefined : $currentUser?.avatarUrl}
        status={isGuest ? undefined : statusDot($presenceStatus)}
      />

      <div class="identity">
        {#if isGuest}
          <h2 class="display-name">Гость</h2>
          <span class="username">Войдите, чтобы профиль сохранялся в аккаунте</span>
        {:else if $currentUser}
          <h2 class="display-name">{$currentUser.displayName}</h2>
          <span class="username">@{$currentUser.username}</span>
          {#if bio}
            <p class="bio">{bio}</p>
          {/if}
          {#if code}
            <div class="code">
              <span class="code-value">Код друга: {code}</span>
              <IconButton label="Скопировать код друга" size="sm" onclick={copyCode}>
                <Copy size="1.5rem" strokeWidth={1.8} />
              </IconButton>
            </div>
          {/if}
          <div class="meta">
            {#if playing}
              <span class="meta-item">
                <Gamepad2 size="1.5rem" strokeWidth={1.8} />
                Играет в {playing.title}
                {#if playingHidden}<HiddenBadge text="Скрыто от других. Вы видите статус, остальные — нет." />{/if}
              </span>
            {/if}
            {#if memberSince}
              <span class="meta-item">
                <Calendar size="1.5rem" strokeWidth={1.8} />
                Участник с {memberSince}
              </span>
            {/if}
            {#if !showOnline}
              <HiddenBadge text="Статус «В сети» скрыт от других. Вы его видите." />
            {/if}
          </div>
        {/if}
      </div>

      <div class="right">
        <div class="head-actions">
          {#if isGuest}
            <Button variant="primary" disabled={busy} onclick={() => run(() => leaveGuest('login'), 'Не удалось открыть вход')}>
              <LogIn size="1.5rem" strokeWidth={1.8} />
              Войти
            </Button>
            <Button disabled={busy} onclick={() => run(() => leaveGuest('register'), 'Не удалось открыть вход')}>
              Создать аккаунт
            </Button>
          {:else}
            <AvatarEditor size="sm" disabled={$isOffline} />
            <DropdownMenu items={menuItems} onselect={onMenu}>
              {#snippet trigger({ toggle })}
                <IconButton label="Ещё" onclick={toggle}>
                  <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
                </IconButton>
              {/snippet}
            </DropdownMenu>
          {/if}
        </div>
        {#if !isGuest}
          <div class="stats">
            <ProfileStats {stats} hidden={!showStats} />
          </div>
        {/if}
      </div>
    </div>

    {#if editing && $currentUser}
      <div class="fields">
        <label class="field">
          <span class="field-label">Отображаемое имя</span>
          <input class="input" type="text" maxlength="32" disabled={$isOffline} bind:value={draft.displayName} />
          {#if fieldErrors.displayName}<span class="error">{fieldErrors.displayName}</span>{/if}
        </label>
        <label class="field">
          <span class="field-label">Имя пользователя</span>
          <div class="username-field">
            <span class="username-prefix">@</span>
            <input class="input" type="text" maxlength="24" disabled={$isOffline} bind:value={draft.username} />
          </div>
          {#if fieldErrors.username}<span class="error">{fieldErrors.username}</span>{/if}
        </label>
        <label class="field">
          <span class="field-label">
            О себе
            <span class="counter">{draft.bio.length}/{BIO_LIMIT}</span>
          </span>
          <textarea
            class="input area"
            rows="3"
            maxlength={BIO_LIMIT}
            disabled={$isOffline}
            bind:value={draft.bio}
          ></textarea>
          {#if fieldErrors.bio}<span class="error">{fieldErrors.bio}</span>{/if}
        </label>
        <div class="field">
          <span class="field-label">Email</span>
          <MaskedEmail email={$currentUser.email} />
          <span class="hint">Email пока нельзя изменить и его не видит никто, кроме вас</span>
        </div>
        <div class="field">
          <span class="field-label">Участник с</span>
          <span class="hint">{memberSince}</span>
        </div>
      </div>
      <div class="foot">
        {#if fieldErrors.general}<span class="error">{fieldErrors.general}</span>{/if}
        <Button variant="ghost" disabled={$savingProfile} onclick={cancelEditing}>Отмена</Button>
        <Button variant="primary" disabled={!dirty || $savingProfile || $isOffline} onclick={save}>
          {$savingProfile ? 'Сохранение…' : 'Сохранить'}
        </Button>
      </div>
    {/if}
  </Card>
</section>

<style>
  .profile-header {
    display: block;
    margin-bottom: var(--space-6);
  }

  .head {
    display: flex;
    align-items: flex-start;
    gap: var(--space-5);
  }

  .identity {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .display-name {
    font-size: var(--font-title);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
    overflow-wrap: anywhere;
  }

  .username {
    font-size: var(--font-md);
    color: var(--text-3);
  }

  .bio {
    font-size: var(--font-sm);
    color: var(--text-2);
    max-width: 56rem;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .code {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  .code-value {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-4);
    margin-top: 0.4rem;
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .meta-item :global(svg) {
    color: var(--text-3);
    flex-shrink: 0;
  }

  .right {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--space-4);
    flex-shrink: 0;
  }

  .head-actions {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    flex-shrink: 0;
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 44rem;
    margin-top: var(--space-5);
    padding-top: var(--space-5);
    border-top: 1px solid var(--border);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .field-label {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.8rem;
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .counter {
    color: var(--text-3);
  }

  .input {
    height: var(--control-md);
    padding: 0 1.2rem;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    color: var(--text);
    font-size: var(--font-sm);
    font-family: inherit;
    width: 100%;
    transition:
      border-color var(--dur) var(--ease),
      box-shadow var(--dur) var(--ease);
  }

  .input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-subtle);
  }

  .area {
    height: auto;
    padding: 0.9rem 1.2rem;
    resize: vertical;
    line-height: 1.5;
  }

  .username-field {
    position: relative;
    display: flex;
    align-items: center;
  }

  .username-prefix {
    position: absolute;
    left: 1.2rem;
    color: var(--text-3);
    font-size: var(--font-sm);
    pointer-events: none;
  }

  .username-field .input {
    padding-left: 2.6rem;
  }

  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .error {
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .foot {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.8rem;
    margin-top: var(--space-5);
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
  }

  .foot .error {
    margin-right: auto;
  }

  @media (max-width: 1200px) {
    .head {
      flex-wrap: wrap;
    }

    .right {
      align-items: flex-start;
      width: 100%;
    }
  }
</style>
