<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { AccountError } from '../services/account';
  import { accountErrorText } from '../services/accountMessages';
  import { authState, deleteAccount } from '../stores/user';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let { onclose }: { onclose: () => void } = $props();

  let password = $state('');
  let running = $state(false);
  let error = $state('');

  function failureText(err: unknown): string {
    if (err instanceof AccountError && err.code === 'invalid_credentials') return msg('profile.deleteAccountWrongPassword');
    return accountErrorText(err, msg('profile.deleteAccountFailed'));
  }

  async function confirm() {
    if (running || !password) return;
    running = true;
    error = '';
    try {
      await deleteAccount(password);
      toast(msg('profile.deleteAccountDone'), 'success');
      onclose();
    } catch (err) {
      const text = failureText(err);
      if ($authState === 'authenticated' || $authState === 'offline') {
        error = text;
      } else {
        toast(text, 'danger');
        onclose();
      }
    } finally {
      running = false;
    }
  }
</script>

<Modal open title={msg('profile.deleteAccountConfirmTitle')} width="44rem" {onclose}>
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void confirm();
    }}
  >
    <p class="text">{msg('profile.deleteAccountConfirmText')}</p>
    <label class="field">
      <span class="field-label">{msg('profile.deleteAccountPassword')}</span>
      <input class="input" type="password" autocomplete="off" disabled={running} bind:value={password} />
    </label>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </form>
  {#snippet footer()}
    <Button disabled={running} onclick={onclose}>{msg('common.cancel')}</Button>
    <Button variant="danger" disabled={running || !password} onclick={confirm}>
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
