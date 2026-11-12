<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { accountErrorField, accountErrorText, accountMessage } from '../../lib/services/accountMessages';
  import { openForgotPassword, openSupportSite } from '../../lib/services/account';
  import { authReason, authState, authView, enterAsGuest, retryBootstrap, signIn, signUp } from '../../lib/stores/user';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';

  const uid = $props.id();

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
    if (busy) return;
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

  async function onForgot() {
    try {
      await openForgotPassword();
    } catch (err) {
      console.error('open password reset page', err);
      toast(msg('tickets.forgotOpenFailed'), 'danger');
    }
  }

  async function onCantSignIn() {
    try {
      await openSupportSite();
    } catch (err) {
      console.error('open support page', err);
      toast(msg('tickets.siteOpenFailed'), 'danger');
    }
  }

  function onSubmit(event: SubmitEvent) {
    event.preventDefault();
    if (isRegister) submitRegister();
    else submitLogin();
  }
</script>

<div class="auth" style="--wails-draggable: drag">
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
          <div class="field">
            <label class="label" for="{uid}-email">{msg('common.email')}</label>
            <input
              id="{uid}-email"
              class="input"
              class:invalid={!!errors.email}
              aria-invalid={errors.email ? 'true' : undefined}
              aria-describedby={errors.email ? `${uid}-email-error` : undefined}
              type="email"
              autocomplete="email"
              required
              bind:value={email}
              disabled={busy}
            />
            {#if errors.email}<span class="error" id="{uid}-email-error" role="alert">{errors.email}</span>{/if}
          </div>

          <div class="field">
            <label class="label" for="{uid}-username">{msg('transfers.authUsernameLabel')}</label>
            <input
              id="{uid}-username"
              class="input"
              class:invalid={!!errors.username}
              aria-invalid={errors.username ? 'true' : undefined}
              aria-describedby={errors.username ? `${uid}-username-error` : undefined}
              type="text"
              maxlength="24"
              autocomplete="username"
              required
              bind:value={username}
              disabled={busy}
            />
            {#if errors.username}<span class="error" id="{uid}-username-error" role="alert">{errors.username}</span>{/if}
          </div>

          <div class="field">
            <label class="label" for="{uid}-displayName">{msg('transfers.authDisplayNameLabel')}</label>
            <input
              id="{uid}-displayName"
              class="input"
              class:invalid={!!errors.displayName}
              aria-invalid={errors.displayName ? 'true' : undefined}
              aria-describedby={errors.displayName ? `${uid}-displayName-error` : undefined}
              type="text"
              maxlength="32"
              autocomplete="nickname"
              required
              bind:value={displayName}
              disabled={busy}
            />
            {#if errors.displayName}<span class="error" id="{uid}-displayName-error" role="alert">{errors.displayName}</span>{/if}
          </div>
        {:else}
          <div class="field">
            <label class="label" for="{uid}-identifier">{msg('transfers.authIdentifierLabel')}</label>
            <input
              id="{uid}-identifier"
              class="input"
              class:invalid={!!errors.username}
              aria-invalid={errors.username ? 'true' : undefined}
              aria-describedby={errors.username ? `${uid}-username-error` : undefined}
              type="text"
              autocomplete="username"
              required
              bind:value={identifier}
              disabled={busy}
            />
            {#if errors.username}<span class="error" id="{uid}-username-error" role="alert">{errors.username}</span>{/if}
          </div>
        {/if}

        <div class="field">
          <label class="label" for="{uid}-password">{msg('transfers.authPasswordLabel')}</label>
          <input
            id="{uid}-password"
            class="input"
            class:invalid={!!errors.password}
            aria-invalid={errors.password ? 'true' : undefined}
            aria-describedby={errors.password ? `${uid}-password-error` : undefined}
            type="password"
            autocomplete={isRegister ? 'new-password' : 'current-password'}
            required
            bind:value={password}
            disabled={busy}
          />
          {#if errors.password}<span class="error" id="{uid}-password-error" role="alert">{errors.password}</span>{/if}
        </div>

        {#if !isRegister}
          <div class="forgot">
            <TextLink disabled={busy} onclick={onForgot}>{msg('tickets.forgotPassword')}</TextLink>
            <TextLink disabled={busy} onclick={onCantSignIn}>{msg('tickets.cantSignIn')}</TextLink>
          </div>
        {/if}

        {#if errors.general}
          <Notice tone="danger" compact>{errors.general}</Notice>
        {/if}

        <div class="submit">
          <Button variant="primary" size="lg" type="submit" block {busy}>
            {#if busy}
              {isRegister ? msg('transfers.authRegistering') : msg('transfers.authSigningIn')}
            {:else}
              {isRegister ? msg('transfers.authCreateAccount') : msg('transfers.authSignInAction')}
            {/if}
          </Button>
        </div>
      </form>

      <p class="switch">
        {#if isRegister}
          {msg('transfers.authHaveAccount')}
          <TextLink disabled={busy} onclick={() => switchView('login')}>{msg('transfers.authSignInAction')}</TextLink>
        {:else}
          {msg('transfers.authNoAccount')}
          <TextLink disabled={busy} onclick={() => switchView('register')}>{msg('transfers.authCreateAccount')}</TextLink>
        {/if}
      </p>
      </div>
      {/key}

      <div class="guest">
        <span class="guest-divider"><span>{msg('transfers.authOrDivider')}</span></span>
        <div class="guest-action">
          <Button block disabled={busy} onclick={onGuest}>{msg('transfers.authGuestAction')}</Button>
        </div>
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
    height: calc(100vh - var(--titlebar-h));
    padding: var(--space-8) 0 var(--space-5);
    overflow-y: auto;
    background:
      radial-gradient(120rem 60rem at 50% -20%, color-mix(in srgb, var(--accent) 16%, transparent), transparent 60%),
      var(--bg);
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

  .submit {
    margin-top: var(--space-2);
  }

  .guest {
    margin-top: var(--space-5);
  }

  .guest-action {
    margin-top: var(--space-3);
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

  .guest-hint {
    margin-top: 0.8rem;
    font-size: var(--font-xs);
    line-height: 1.45;
    color: var(--text-3);
    text-align: center;
  }

  .forgot {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
    margin-top: -0.4rem;
    font-size: var(--font-sm);
  }

  .switch {
    margin-top: var(--space-4);
    font-size: var(--font-sm);
    color: var(--text-3);
    text-align: center;
  }

</style>
