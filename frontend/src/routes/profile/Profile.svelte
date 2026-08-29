<script lang="ts">
  import { LogIn, LogOut, Pencil, UserRound } from '@lucide/svelte';
  import AvatarEditor from '../../lib/components/AvatarEditor.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import MaskedEmail from '../../lib/components/MaskedEmail.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { accountErrorField, accountErrorText } from '../../lib/services/accountMessages';
  import {
    authState,
    currentUser,
    isOffline,
    leaveGuest,
    savingProfile,
    signOut,
    saveProfile,
  } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';

  let editing = $state(false);
  let draft = $state({ displayName: '', username: '' });
  let fieldErrors = $state<{ displayName?: string; username?: string; general?: string }>({});
  let avatarFailed = $state(false);
  let signingOut = $state(false);
  const isGuest = $derived($authState === 'guest');

  $effect(() => {
    $currentUser?.avatarUrl;
    avatarFailed = false;
  });

  const avatarInitial = $derived(
    $currentUser ? ($currentUser.displayName || $currentUser.username).slice(0, 1).toUpperCase() : '?',
  );

  const memberSince = $derived(
    $currentUser
      ? new Date($currentUser.createdAt).toLocaleDateString('ru-RU', {
          day: 'numeric',
          month: 'long',
          year: 'numeric',
        })
      : '',
  );

  const dirty = $derived(
    !!$currentUser &&
      (draft.displayName !== $currentUser.displayName || draft.username !== $currentUser.username),
  );

  function startEditing() {
    if (!$currentUser || $isOffline) return;
    draft = { displayName: $currentUser.displayName, username: $currentUser.username };
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

    const patch: { displayName?: string; username?: string } = {};
    if (draft.displayName !== $currentUser.displayName) patch.displayName = draft.displayName;
    if (draft.username !== $currentUser.username) patch.username = draft.username;

    try {
      await saveProfile(patch);
      editing = false;
      toast('Профиль обновлён', 'success');
    } catch (err) {
      const message = accountErrorText(err, 'Не удалось сохранить');
      const field = accountErrorField(err);
      if (field === 'username') fieldErrors = { username: message };
      else if (field === 'displayName') fieldErrors = { displayName: message };
      else fieldErrors = { general: message };
    }
  }

  async function onSignOut() {
    if (signingOut) return;
    signingOut = true;
    try {
      await signOut();
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось выйти'), 'danger');
    } finally {
      signingOut = false;
    }
  }

  async function toAuth(view: 'login' | 'register') {
    if (signingOut) return;
    signingOut = true;
    try {
      await leaveGuest(view);
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось открыть вход'), 'danger');
    } finally {
      signingOut = false;
    }
  }
</script>

<PageHeader title="Профиль" />

{#if isGuest}
  <EmptyState
    title="Вы вошли как гость"
    description="Библиотека, загрузки и источники работают без аккаунта. Профиль, аватар и синхронизация появятся после входа."
  >
    {#snippet icon()}
      <UserRound size="2rem" strokeWidth={1.8} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" disabled={signingOut} onclick={() => toAuth('login')}>
        <LogIn size="1.5rem" strokeWidth={1.8} />
        Войти
      </Button>
      <Button disabled={signingOut} onclick={() => toAuth('register')}>Создать аккаунт</Button>
    {/snippet}
  </EmptyState>
{:else if !$currentUser}
  <EmptyState title="Нет данных профиля" description="Войдите в аккаунт, чтобы увидеть профиль." />
{:else}
  <div class="profile">
    <Card padding="var(--space-6)">
      <div class="head">
        <div class="avatar">
          {#if avatarFailed || !$currentUser.avatarUrl}
            <span class="avatar-fallback">{avatarInitial}</span>
          {:else}
            <img src={$currentUser.avatarUrl} alt="" draggable="false" onerror={() => (avatarFailed = true)} />
          {/if}
        </div>

        <div class="identity">
          <h2 class="display-name">{$currentUser.displayName}</h2>
          <span class="username">@{$currentUser.username}</span>
          <div class="avatar-actions">
            <AvatarEditor size="sm" disabled={$isOffline} />
          </div>
        </div>

        <div class="head-actions">
          {#if !editing}
            <Button onclick={startEditing} disabled={$isOffline}>
              <Pencil size="1.5rem" strokeWidth={1.8} />
              Редактировать
            </Button>
          {/if}
          <Button variant="danger" disabled={signingOut} onclick={onSignOut}>
            <LogOut size="1.5rem" strokeWidth={1.8} />
            {signingOut ? 'Выход…' : 'Выйти'}
          </Button>
          {#if $isOffline}
            <span class="hint">Изменить профиль можно только при связи с сервером</span>
          {/if}
        </div>
      </div>
    </Card>

    <Card padding="var(--space-6)">
      {#if editing}
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

          <div class="field">
            <span class="field-label">Email</span>
            <MaskedEmail email={$currentUser.email} />
            <span class="hint">Email пока нельзя изменить и его не видит никто, кроме вас</span>
          </div>
        </div>

        <div class="foot">
          {#if fieldErrors.general}<span class="error">{fieldErrors.general}</span>{/if}
          <Button variant="ghost" disabled={$savingProfile} onclick={cancelEditing}>Отмена</Button>
          <Button variant="primary" disabled={!dirty || $savingProfile || $isOffline} onclick={save}>
            {$savingProfile ? 'Сохранение…' : 'Сохранить'}
          </Button>
        </div>
      {:else}
        <dl class="facts">
          <div class="fact">
            <dt>Отображаемое имя</dt>
            <dd>{$currentUser.displayName}</dd>
          </div>
          <div class="fact">
            <dt>Имя пользователя</dt>
            <dd>@{$currentUser.username}</dd>
          </div>
          <div class="fact">
            <dt>Email</dt>
            <dd><MaskedEmail email={$currentUser.email} /></dd>
          </div>
          <div class="fact">
            <dt>Участник с</dt>
            <dd>{memberSince}</dd>
          </div>
        </dl>
      {/if}
    </Card>
  </div>
{/if}

<style>
  .profile {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: var(--prose-max);
  }

  .head {
    display: flex;
    align-items: flex-start;
    gap: var(--space-5);
  }

  .avatar {
    width: 9.6rem;
    height: 9.6rem;
    flex-shrink: 0;
  }

  .avatar img {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    object-fit: cover;
  }

  .avatar-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--text-2);
    font-size: 3.6rem;
    font-weight: 600;
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

  .avatar-actions {
    display: flex;
    gap: 0.8rem;
    margin-top: var(--space-3);
    flex-wrap: wrap;
  }

  .head-actions {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    flex-shrink: 0;
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 44rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-2);
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

  .facts {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(22rem, 1fr));
    gap: var(--space-5);
  }

  .fact {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-width: 0;
  }

  .fact dt {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .fact dd {
    font-size: var(--font-md);
    color: var(--text);
    overflow-wrap: anywhere;
  }

  @media (max-width: 1240px) {
    .head {
      flex-wrap: wrap;
    }

    .head-actions {
      flex-direction: row;
      width: 100%;
    }
  }
</style>
