<script lang="ts">
  import { tick } from 'svelte';
  import { Eye, EyeOff } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import TextField from '../../lib/components/TextField.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import { accountErrorField, accountErrorText, accountMessage } from '../../lib/services/accountMessages';
  import { openForgotPassword, openSupportSite } from '../../lib/services/account';
  import { authReason, authState, authView, enterAsGuest, retryBootstrap, signIn, signUp } from '../../lib/stores/user';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { msg } from '../../lib/i18n';

  const uid = $props.id();

  type Field = 'email' | 'username' | 'displayName' | 'password';
  type FieldErrors = Partial<Record<Field | 'general', string>>;

  // The fields each form has, first to last: a refusal about one of them is said at it, any other in
  // the form. On the sign-in form the login field takes what the server says about a username.
  const FIELDS: Record<'login' | 'register', Field[]> = {
    login: ['username', 'password'],
    register: ['email', 'username', 'displayName', 'password'],
  };

  let email = $state('');
  let username = $state('');
  let displayName = $state('');
  let password = $state('');
  let identifier = $state('');
  let errors = $state<FieldErrors>({});
  let revealed = $state(false);
  // Each request shows on its own button; either one holds the whole form.
  let submitting = $state(false);
  let guesting = $state(false);
  const busy = $derived(submitting || guesting);

  const isRegister = $derived($authView === 'register');
  const offline = $derived($authState === 'unavailable');

  const fieldId = (field: Field) => (field === 'username' && !isRegister ? `${uid}-identifier` : `${uid}-${field}`);

  // The pressed link goes with the form it was under: focus it held moves to the first field of the
  // new one.
  function switchView(view: 'login' | 'register') {
    if (busy) return;
    const hand = focusHandOff();
    errors = {};
    password = '';
    revealed = false;
    authView.set(view);
    void hand(() => document.getElementById(view === 'register' ? `${uid}-email` : `${uid}-identifier`));
  }

  // Focus goes to the field the refusal is about; a refusal of the whole form leaves it on the button
  // or in the field Enter was pressed in.
  async function applyError(err: unknown) {
    const named = accountErrorField(err);
    const field = FIELDS[isRegister ? 'register' : 'login'].find((shown) => shown === named);
    const message = accountErrorText(err, msg('transfers.authGenericError'));
    if (!field) {
      errors = { general: message };
      return;
    }
    errors = { [field]: message };
    await tick();
    document.getElementById(fieldId(field))?.focus();
  }

  async function submitRegister() {
    if (busy) return;
    errors = {};
    submitting = true;
    try {
      await signUp({ email, username, displayName, password });
    } catch (err) {
      await applyError(err);
    } finally {
      submitting = false;
    }
  }

  async function submitLogin() {
    if (busy) return;
    errors = {};
    submitting = true;
    try {
      await signIn({ emailOrUsername: identifier, password });
    } catch (err) {
      await applyError(err);
    } finally {
      submitting = false;
    }
  }

  async function onGuest() {
    if (busy) return;
    errors = {};
    guesting = true;
    try {
      await enterAsGuest();
    } catch (err) {
      errors = { general: accountErrorText(err, msg('transfers.authGuestError')) };
    } finally {
      guesting = false;
    }
  }

  // Said in the form: the toast stack lives in the shell, which is not on screen before sign-in.
  async function onForgot() {
    errors = {};
    try {
      await openForgotPassword();
    } catch (err) {
      console.error('open password reset page', err);
      errors = { general: msg('tickets.forgotOpenFailed') };
    }
  }

  async function onCantSignIn() {
    errors = {};
    try {
      await openSupportSite();
    } catch (err) {
      console.error('open support page', err);
      errors = { general: msg('tickets.siteOpenFailed') };
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

      <!-- While a request runs the fields refuse edits by being read-only, not disabled: a field that
           turns disabled loses the focus Enter was pressed in. -->
      <form class="form" onsubmit={onSubmit} aria-busy={busy || undefined}>
        {#if isRegister}
          <TextField
            id={fieldId('email')}
            label={msg('common.email')}
            type="email"
            autocomplete="email"
            required
            readonly={busy}
            error={errors.email}
            bind:value={email}
          />
          <TextField
            id={fieldId('username')}
            label={msg('transfers.authUsernameLabel')}
            maxlength={24}
            autocomplete="username"
            required
            readonly={busy}
            error={errors.username}
            bind:value={username}
          />
          <TextField
            id={fieldId('displayName')}
            label={msg('transfers.authDisplayNameLabel')}
            maxlength={32}
            autocomplete="nickname"
            required
            readonly={busy}
            error={errors.displayName}
            bind:value={displayName}
          />
        {:else}
          <TextField
            id={fieldId('username')}
            label={msg('transfers.authIdentifierLabel')}
            autocomplete="username"
            required
            readonly={busy}
            error={errors.username}
            bind:value={identifier}
          />
        {/if}

        <TextField
          id={fieldId('password')}
          label={msg('transfers.authPasswordLabel')}
          type={revealed ? 'text' : 'password'}
          autocomplete={isRegister ? 'new-password' : 'current-password'}
          required
          readonly={busy}
          error={errors.password}
          bind:value={password}
        >
          {#snippet trailing()}
            <IconButton size="sm" label={msg('transfers.authShowPassword')} pressed={revealed} onclick={() => (revealed = !revealed)}>
              {#if revealed}<EyeOff size="1.6rem" strokeWidth={1.8} />{:else}<Eye size="1.6rem" strokeWidth={1.8} />{/if}
            </IconButton>
          {/snippet}
        </TextField>

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
          <Button variant="primary" size="lg" type="submit" block busy={submitting} disabled={guesting}>
            {#if submitting}
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
          <Button block busy={guesting} disabled={submitting} onclick={onGuest}>{msg('transfers.authGuestAction')}</Button>
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
