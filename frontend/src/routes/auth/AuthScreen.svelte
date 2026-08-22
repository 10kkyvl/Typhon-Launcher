<script lang="ts">
  import { Minus, Square, X } from '@lucide/svelte';
  import { Window } from '@wailsio/runtime';
  import Button from '../../lib/components/Button.svelte';
  import { accountErrorField, accountErrorText, accountMessage } from '../../lib/services/accountMessages';
  import { inWails } from '../../lib/services/backend';
  import { authReason, authState, authView, enterAsGuest, retryBootstrap, signIn, signUp } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';

  type FieldErrors = Partial<Record<'email' | 'username' | 'displayName' | 'password' | 'general', string>>;

  let email = $state('');
  let username = $state('');
  let displayName = $state('');
  let password = $state('');
  let identifier = $state('');
  let errors = $state<FieldErrors>({});
  let busy = $state(false);

  const isRegister = $derived($authView === 'register');
  const offline = $derived($authState === 'unavailable');

  function switchView(view: 'login' | 'register') {
    errors = {};
    password = '';
    authView.set(view);
  }

  function applyError(err: unknown) {
    const field = accountErrorField(err);
    const message = accountErrorText(err, 'Не удалось выполнить вход');
    switch (field) {
      case 'email':
      case 'username':
      case 'displayName':
      case 'password':
        errors = { [field]: message };
        return;
      default:
        errors = { general: message };
    }
  }

  async function submitRegister() {
    if (busy) return;
    errors = {};
    busy = true;
    try {
      await signUp({ email, username, displayName, password });
    } catch (err) {
      applyError(err);
    } finally {
      busy = false;
    }
  }

  async function submitLogin() {
    if (busy) return;
    errors = {};
    busy = true;
    try {
      await signIn({ emailOrUsername: identifier, password });
    } catch (err) {
      applyError(err);
    } finally {
      busy = false;
    }
  }

  async function onGuest() {
    if (busy) return;
    errors = {};
    busy = true;
    try {
      await enterAsGuest();
    } catch (err) {
      errors = { general: accountErrorText(err, 'Не удалось продолжить без аккаунта') };
    } finally {
      busy = false;
    }
  }

  function onSubmit(event: SubmitEvent) {
    event.preventDefault();
    if (isRegister) submitRegister();
    else submitLogin();
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

<div class="auth" style="--wails-draggable: drag">
  <div class="window-controls" style="--wails-draggable: no-drag">
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

  <div class="panel" style="--wails-draggable: no-drag">
    <div class="brand">
      <img class="brand-mark" src="/typhon.svg" alt="" draggable="false" />
      <span class="brand-name">Typhon</span>
    </div>

    {#if offline}
      <h1 class="title">Нет связи с сервером</h1>
      <p class="subtitle">{accountMessage($authReason, 'Сервер аккаунтов недоступен')}</p>
      <div class="offline-actions">
        <Button variant="primary" size="lg" onclick={() => retryBootstrap()}>Повторить</Button>
      </div>
      <p class="note">Сохранённая сессия не удалена — она восстановится, когда сервер снова ответит.</p>
    {:else}
      <h1 class="title">{isRegister ? 'Создание аккаунта' : 'Вход в Typhon'}</h1>

      <form class="form" onsubmit={onSubmit}>
        {#if isRegister}
          <label class="field">
            <span class="label">Email</span>
            <input
              class="input"
              class:invalid={!!errors.email}
              type="email"
              autocomplete="email"
              bind:value={email}
              disabled={busy}
            />
            {#if errors.email}<span class="error">{errors.email}</span>{/if}
          </label>

          <label class="field">
            <span class="label">Имя пользователя</span>
            <input
              class="input"
              class:invalid={!!errors.username}
              type="text"
              maxlength="24"
              autocomplete="username"
              bind:value={username}
              disabled={busy}
            />
            {#if errors.username}<span class="error">{errors.username}</span>{/if}
          </label>

          <label class="field">
            <span class="label">Отображаемое имя</span>
            <input
              class="input"
              class:invalid={!!errors.displayName}
              type="text"
              maxlength="32"
              autocomplete="nickname"
              bind:value={displayName}
              disabled={busy}
            />
            {#if errors.displayName}<span class="error">{errors.displayName}</span>{/if}
          </label>
        {:else}
          <label class="field">
            <span class="label">Email или имя пользователя</span>
            <input
              class="input"
              class:invalid={!!errors.username}
              type="text"
              autocomplete="username"
              bind:value={identifier}
              disabled={busy}
            />
            {#if errors.username}<span class="error">{errors.username}</span>{/if}
          </label>
        {/if}

        <label class="field">
          <span class="label">Пароль</span>
          <input
            class="input"
            class:invalid={!!errors.password}
            type="password"
            autocomplete={isRegister ? 'new-password' : 'current-password'}
            bind:value={password}
            disabled={busy}
          />
          {#if errors.password}<span class="error">{errors.password}</span>{/if}
        </label>

        {#if errors.general}
          <p class="error general">{errors.general}</p>
        {/if}

        <button class="submit" type="submit" disabled={busy}>
          {#if busy}
            {isRegister ? 'Создание…' : 'Вход…'}
          {:else}
            {isRegister ? 'Создать аккаунт' : 'Войти'}
          {/if}
        </button>
      </form>

      <div class="guest">
        <span class="guest-divider"><span>или</span></span>
        <button class="guest-btn" type="button" disabled={busy} onclick={onGuest}>Войти как гость</button>
        <p class="guest-hint">Библиотека, загрузки и источники работают локально. Профиль и аватар — только с аккаунтом.</p>
      </div>

      <p class="switch">
        {#if isRegister}
          Уже есть аккаунт?
          <button class="link" type="button" onclick={() => switchView('login')}>Войти</button>
        {:else}
          Нет аккаунта?
          <button class="link" type="button" onclick={() => switchView('register')}>Создать аккаунт</button>
        {/if}
      </p>
    {/if}
  </div>
</div>

<style>
  .auth {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    background:
      radial-gradient(120rem 60rem at 50% -20%, rgba(104, 117, 232, 0.16), transparent 60%),
      var(--bg);
  }

  .window-controls {
    position: absolute;
    top: 0.8rem;
    right: 0.8rem;
    display: flex;
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

  .panel {
    width: 40rem;
    padding: var(--space-8);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-modal);
    animation: rise var(--dur-panel) var(--ease);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 1rem;
    margin-bottom: var(--space-6);
  }

  .brand-mark {
    width: 3.2rem;
    height: 3.2rem;
  }

  .brand-name {
    font-size: 2.1rem;
    font-weight: 600;
    letter-spacing: var(--tracking-title);
  }

  .title {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .subtitle {
    margin-top: 0.8rem;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .note {
    margin-top: var(--space-4);
    font-size: var(--font-xs);
    color: var(--text-3);
    line-height: 1.5;
  }

  .offline-actions {
    margin-top: var(--space-5);
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-5);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .label {
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
    transition:
      border-color var(--dur) var(--ease),
      box-shadow var(--dur) var(--ease);
  }

  .input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-subtle);
  }

  .input.invalid {
    border-color: var(--danger);
  }

  .input:disabled {
    opacity: 0.6;
  }

  .error {
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .error.general {
    margin-top: -0.4rem;
  }

  .submit {
    height: var(--control-lg);
    margin-top: var(--space-2);
    background: var(--accent);
    color: #fff;
    font-size: var(--font-md);
    font-weight: 600;
    border-radius: var(--cut) var(--radius-md) var(--radius-md) var(--radius-md);
    transition:
      background var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .submit:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .submit:active:not(:disabled) {
    transform: translateY(1px);
  }

  .submit:disabled {
    opacity: 0.55;
    cursor: default;
  }

  .guest {
    margin-top: var(--space-5);
  }

  .guest-divider {
    display: flex;
    align-items: center;
    gap: 1rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .guest-divider::before,
  .guest-divider::after {
    content: "";
    flex: 1;
    height: 1px;
    background: var(--border-strong);
  }

  .guest-btn {
    width: 100%;
    height: var(--control-md);
    margin-top: var(--space-3);
    background: var(--surface-3);
    color: var(--text);
    font-size: var(--font-sm);
    font-weight: 500;
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .guest-btn:hover:not(:disabled) {
    background: var(--surface-4);
  }

  .guest-btn:disabled {
    opacity: 0.55;
    cursor: default;
  }

  .guest-hint {
    margin-top: 0.8rem;
    font-size: var(--font-xs);
    line-height: 1.45;
    color: var(--text-3);
    text-align: center;
  }

  .switch {
    margin-top: var(--space-5);
    font-size: var(--font-sm);
    color: var(--text-3);
    text-align: center;
  }

  .link {
    color: var(--accent-text);
    font-size: var(--font-sm);
    font-weight: 500;
    padding: 0.2rem 0.3rem;
    border-radius: var(--radius-xs);
  }

  .link:hover {
    color: var(--text);
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(0.8rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
