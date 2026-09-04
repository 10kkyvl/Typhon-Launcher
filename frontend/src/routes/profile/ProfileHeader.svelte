<script lang="ts">
  import { Copy, EllipsisVertical, LogIn } from '@lucide/svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import AvatarEditor from '../../lib/components/AvatarEditor.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import MaskedEmail from '../../lib/components/MaskedEmail.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { accountErrorField, accountErrorText } from '../../lib/services/accountMessages';
  import type { GameRef } from '../../lib/services/profile';
  import { friendCode } from '../../lib/services/social';
  import { statusLine } from '../../lib/profile/view';
  import { dotKind, ownStatusLine, statusDot } from '../../lib/social/presence';
  import { joinDate } from '../../lib/social/view';
  import { presenceStatus } from '../../lib/stores/presence';
  import { settings } from '../../lib/stores/settings';
  import { authState, currentUser, isOffline, leaveGuest, saveProfile, savingProfile, signOut } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';

  type MenuItem = { id: string; label: string; danger?: boolean; separator?: boolean };

  let {
    running,
    showOnline,
    showPlaying,
    onsettings,
  }: {
    running: GameRef[];
    showOnline: boolean;
    showPlaying: boolean;
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
  const online = $derived($authState === 'authenticated');
  const status = $derived(statusLine(running, online));
  const own = $derived(!isGuest && online ? ownStatusLine($presenceStatus, status.kind === 'playing') : '');
  const statusText = $derived(own || status.text);
  const statusKind = $derived(
    own ? dotKind(statusDot($presenceStatus)) : status.kind === 'offline' ? 'neutral' : 'success',
  );
  const statusHidden = $derived(!isGuest && (status.kind === 'playing' ? !showPlaying : !showOnline));

  const avatarName = $derived(
    !isGuest && $currentUser ? $currentUser.displayName || $currentUser.username : 'Гость',
  );

  const memberSince = $derived($currentUser ? joinDate($currentUser.createdAt) : '');

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
      <Avatar size="lg" name={avatarName} src={isGuest ? undefined : $currentUser?.avatarUrl} />

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
        {/if}
        <div class="status">
          <StatusBadge plain kind={statusKind} label={statusText} />
          {#if statusHidden}
            <span class="muted">· скрыто от других</span>
          {/if}
        </div>
        {#if !isGuest}
          <div class="avatar-actions">
            <AvatarEditor size="sm" disabled={$isOffline} />
          </div>
        {/if}
      </div>

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
          <DropdownMenu items={menuItems} onselect={onMenu}>
            {#snippet trigger({ toggle })}
              <IconButton label="Ещё" onclick={toggle}>
                <EllipsisVertical size="1.8rem" strokeWidth={1.8} />
              </IconButton>
            {/snippet}
          </DropdownMenu>
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
    margin-bottom: var(--space-10);
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

  .status {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }

  .muted {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .avatar-actions {
    display: flex;
    gap: 0.8rem;
    margin-top: var(--space-3);
    flex-wrap: wrap;
  }

  .head-actions {
    display: flex;
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
  }
</style>
