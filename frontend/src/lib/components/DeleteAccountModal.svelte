<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { AccountError } from '../services/account';
  import { accountErrorText } from '../services/accountMessages';
  import { authState, deleteAccount } from '../stores/user';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let { onclose }: { onclose: () => void } = $props();
  const uid = $props.id();

  let password = $state('');
  let running = $state(false);
  let error = $state('');
  // The refusal was the password itself: it is said by the field, which is marked wrong.
  let wrongPassword = $state(false);

  const isWrongPassword = (err: unknown) => err instanceof AccountError && err.code === 'invalid_credentials';

  function failureText(err: unknown): string {
    if (isWrongPassword(err)) return msg('profile.deleteAccountWrongPassword');
    return accountErrorText(err, msg('profile.deleteAccountFailed'));
  }

  async function confirm() {
    if (running || !password) return;
    running = true;
    error = '';
    wrongPassword = false;
    try {
      await deleteAccount(password);
      toast(msg('profile.deleteAccountDone'), 'success');
      onclose();
    } catch (err) {
      const text = failureText(err);
      if ($authState === 'authenticated' || $authState === 'offline') {
        error = text;
        wrongPassword = isWrongPassword(err);
      } else {
        toast(text, 'danger');
        onclose();
      }
    } finally {
      running = false;
    }
  }
</script>

<Modal dismissible={!running} open title={msg('profile.deleteAccountConfirmTitle')} width="44rem" {onclose}>
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void confirm();
    }}
  >
    <p class="text">{msg('profile.deleteAccountConfirmText')}</p>
    <div class="field">
      <label class="field-label" for="{uid}-password">{msg('profile.deleteAccountPassword')}</label>
      <input
        id="{uid}-password"
        class="input"
        type="password"
        autocomplete="off"
        readonly={running}
        aria-invalid={wrongPassword ? 'true' : undefined}
        aria-describedby={error && wrongPassword ? `${uid}-error` : undefined}
        bind:value={password}
      />
      {#if error && wrongPassword}<p class="error" id="{uid}-error" role="alert">{error}</p>{/if}
    </div>
    {#if error && !wrongPassword}<Notice tone="danger" compact>{error}</Notice>{/if}
  </form>
  {#snippet footer()}
    <Button disabled={running} onclick={onclose}>{msg('common.cancel')}</Button>
    <Button variant="danger" disabled={!running && !password} busy={running} onclick={confirm}>
      {running ? msg('profile.deleteAccountBusy') : msg('profile.deleteAccountConfirmButton')}
    </Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .text {
    color: var(--text-2);
    line-height: 1.55;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .field-label {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
  }

  .error {
    font-size: var(--font-sm);
    color: var(--danger);
  }
</style>
