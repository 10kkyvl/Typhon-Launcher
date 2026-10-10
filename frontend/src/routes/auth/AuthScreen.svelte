<script lang="ts">
  import { CircleAlert, Minus, Square, X } from '@lucide/svelte';
  import { Window } from '@wailsio/runtime';
  import Button from '../../lib/components/Button.svelte';
  import { accountErrorField, accountErrorText, accountMessage } from '../../lib/services/accountMessages';
  import { inWails } from '../../lib/services/backend';
  import { authReason, authState, authView, enterAsGuest, retryBootstrap, signIn, signUp } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';

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
    const message = accountErrorText(err, msg('transfers.authGenericError'));
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
      errors = { general: accountErrorText(err, msg('transfers.authGuestError')) };
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
      toast(msg('transfers.authDesktopOnly'));
      return;
    }
    if (action === 'minimise') Window.Minimise();
    else if (action === 'maximise') Window.ToggleMaximise();
    else Window.Close();
  }
</script>

<div class="auth" style="--wails-draggable: drag">
  <div class="window-controls" style="--wails-draggable: no-drag">
    <button class="wc" aria-label={msg('transfers.authMinimizeLabel')} onclick={() => win('minimise')}>
      <Minus size="1.6rem" strokeWidth={1.6} />
    </button>
    <button class="wc" aria-label={msg('transfers.authMaximizeLabel')} onclick={() => win('maximise')}>
      <Square size="1.2rem" strokeWidth={1.6} />
    </button>
    <button class="wc close" aria-label={msg('common.close')} onclick={() => win('close')}>
      <X size="1.6rem" strokeWidth={1.6} />
    </button>
  </div>

  <div class="panel" style="--wails-draggable: no-drag">
    <div class="brand">
      <img class="brand-mark" src="/typhon.png" alt="" draggable="false" />
      <span class="brand-name">Typhon</span>
    </div>

    {#if offline}
      <h1 class="title">{msg('transfers.authOfflineTitle')}</h1>
      <p class="subtitle">{accountMessage($authReason, msg('transfers.authServerUnavailable'))}</p>
      <div class="offline-actions">
        <Button variant="primary" size="lg" onclick={() => retryBootstrap()}>{msg('common.retry')}</Button>
      </div>
      <p class="note">{msg('transfers.authOfflineNote')}</p>
    {:else}
      {#key isRegister}
      <div class="view">
      <h1 class="title">{isRegister ? msg('transfers.authRegisterTitle') : msg('transfers.authLoginTitle')}</h1>

      <form class="form" onsubmit={onSubmit}>
        {#if isRegister}
          <label class="field">
            <span class="label">{msg('common.email')}</span>
            <input
              class="input"
              class:invalid={!!errors.email}
              type="email"
              autocomplete="email"
              bind:value={email}
              disabled={busy}
            />
            {#if errors.email}<span class="error" role="alert">{errors.email}</span>{/if}
          </label>

          <label class="field">
            <span class="label">{msg('transfers.authUsernameLabel')}</span>
            <input
              class="input"
              class:invalid={!!errors.username}
              type="text"
              maxlength="24"
              autocomplete="username"
              bind:value={username}
              disabled={busy}
            />
            {#if errors.username}<span class="error" role="alert">{errors.username}</span>{/if}
          </label>

          <label class="field">
            <span class="label">{msg('transfers.authDisplayNameLabel')}</span>
            <input
              class="input"
              class:invalid={!!errors.displayName}
              type="text"
              maxlength="32"
              autocomplete="nickname"
              bind:value={displayName}
              disabled={busy}
            />
            {#if errors.displayName}<span class="error" role="alert">{errors.displayName}</span>{/if}
          </label>
        {:else}
          <label class="field">
            <span class="label">{msg('transfers.authIdentifierLabel')}</span>
            <input
              class="input"
              class:invalid={!!errors.username}
              type="text"
              autocomplete="username"
              bind:value={identifier}
              disabled={busy}
            />
            {#if errors.username}<span class="error" role="alert">{errors.username}</span>{/if}
          </label>
        {/if}

        <label class="field">
          <span class="label">{msg('transfers.authPasswordLabel')}</span>
          <input
            class="input"
            class:invalid={!!errors.password}
            type="password"
            autocomplete={isRegister ? 'new-password' : 'current-password'}
            bind:value={password}
            disabled={busy}
          />
          {#if errors.password}<span class="error" role="alert">{errors.password}</span>{/if}
        </label>

        {#if errors.general}
          <p class="error general" role="alert">
            <CircleAlert size="1.6rem" strokeWidth={1.8} />
            <span>{errors.general}</span>
          </p>
        {/if}

        <button class="submit" type="submit" disabled={busy}>
          {#if busy}
            {isRegister ? msg('transfers.authRegistering') : msg('transfers.authSigningIn')}
          {:else}
            {isRegister ? msg('transfers.authCreateAccount') : msg('transfers.authSignInAction')}
          {/if}
        </button>
      </form>

      <p class="switch">
        {#if isRegister}
          {msg('transfers.authHaveAccount')}
          <button class="link" type="button" onclick={() => switchView('login')}>{msg('transfers.authSignInAction')}</button>
        {:else}
          {msg('transfers.authNoAccount')}
          <button class="link" type="button" onclick={() => switchView('register')}>{msg('transfers.authCreateAccount')}</button>
        {/if}
      </p>
      </div>
      {/key}

      <div class="guest">
        <span class="guest-divider"><span>{msg('transfers.authOrDivider')}</span></span>
        <button class="guest-btn" type="button" disabled={busy} onclick={onGuest}>{msg('transfers.authGuestAction')}</button>
        <p class="guest-hint">{msg('transfers.authGuestHint')}</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .auth {
    position: relative;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    height: 100vh;
    padding: var(--space-8) 0 var(--space-5);
    overflow-y: auto;
    background:
      radial-gradient(120rem 60rem at 50% -20%, color-mix(in srgb, var(--accent) 16%, transparent), transparent 60%),
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
    background: var(--danger);
    color: var(--bg);
  }

  .panel {
    width: 40rem;
    margin: auto;
    padding: var(--space-6) var(--space-8);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-modal);
    animation: rise-in var(--dur-panel) var(--ease);
  }

  .view {
    animation: media-in var(--dur-panel) var(--ease);
  }

  .brand {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 1rem;
    margin-bottom: var(--space-4);
  }

  .brand-mark {
    width: 3.6rem;
    height: 3.6rem;
  }

  .brand-name {
    font-size: 2.1rem;
    font-weight: 600;
    letter-spacing: var(--tracking-title);
  }

  .title {
    font-size: var(--font-lg);
    font-weight: 500;
    letter-spacing: var(--tracking-heading);
    color: var(--text-2);
    text-align: center;
  }

  .subtitle {
    margin-top: 0.8rem;
    font-size: var(--font-sm);
    color: var(--text-2);
    text-align: center;
  }

  .note {
    margin-top: var(--space-4);
    font-size: var(--font-xs);
    color: var(--text-3);
    line-height: 1.5;
    text-align: center;
  }

  .offline-actions {
    display: flex;
    justify-content: center;
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
    display: flex;
    align-items: flex-start;
    gap: 0.8rem;
    padding: 0.9rem 1.2rem;
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
    font-size: var(--font-sm);
    line-height: 1.4;
    animation: rise-in var(--dur-panel) var(--ease);
  }

  .error.general :global(svg) {
    flex-shrink: 0;
    margin-top: 0.1rem;
  }

  .submit {
    height: var(--control-lg);
    margin-top: var(--space-2);
    background: var(--accent);
    color: var(--accent-on, #fff);
    font-size: var(--font-md);
    font-weight: 600;
    border-radius: var(--radius-md);
    transition:
      background var(--dur) var(--ease),
      box-shadow var(--dur) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .submit:hover:not(:disabled) {
    background: var(--accent-hover);
    box-shadow: 0 0.8rem 2rem -0.8rem var(--accent-ring);
    transform: translateY(-1px);
  }

  .submit:active:not(:disabled) {
    transform: scale(0.98);
    box-shadow: none;
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
    background: transparent;
    border: 1px solid var(--border-strong);
    color: var(--text-2);
    font-size: var(--font-sm);
    font-weight: 500;
    border-radius: var(--radius-md);
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .guest-btn:hover:not(:disabled) {
    background: var(--hover-strong);
    color: var(--text);
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
    margin-top: var(--space-4);
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
</style>
