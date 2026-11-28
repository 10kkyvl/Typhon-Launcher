<script lang="ts">
  import { onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import TextField from './TextField.svelte';
  import { AccountError } from '../services/account';
  import { accountErrorText } from '../services/accountMessages';
  import { authState, currentUser, deleteAccount } from '../stores/user';
  import { toast } from '../stores/toasts';
  import { focusHandOff } from '../utils/focusHandOff';
  import { msg } from '../i18n';

  /**
   * Deletes the signed-in account for good, after its password. Delete is never the first focus and is
   * held until a password is typed; while the deletion runs nothing in the dialog acts and it cannot be
   * dismissed. The profile settings mount one instance per press, and a session that ends swaps the
   * app's whole tree for the sign-in screen, this dialog with it, before another account can sign in:
   * an answer under another account can only reach an instance that is gone, which says it in a toast
   * naming the account that pressed.
   */
  let { onclose }: { onclose: () => void } = $props();
  const uid = $props.id();
  const fieldId = `${uid}-password`;
  const failureId = `${uid}-failure`;

  let password = $state('');
  let running = $state(false);
  let error = $state('');
  // The refusal was the password itself: it is said by the field, which is marked wrong.
  let wrongPassword = $state(false);
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  const isWrongPassword = (err: unknown) => err instanceof AccountError && err.code === 'invalid_credentials';

  function failureText(err: unknown): string {
    if (isWrongPassword(err)) return msg('profile.deleteAccountWrongPassword');
    return accountErrorText(err, msg('profile.deleteAccountFailed'));
  }

  async function confirm() {
    if (running || !password) return;
    const presser = get(currentUser);
    // The focus the press holds (Delete, or the field for Enter) goes to the words of a refusal.
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    running = true;
    error = '';
    wrongPassword = false;
    try {
      await deleteAccount(password);
      toast(msg('profile.deleteAccountDone'), 'success');
      if (alive) onclose();
      return;
    } catch (err) {
      const text = failureText(err);
      const signedIn = get(authState) === 'authenticated' || get(authState) === 'offline';
      const owner = signedIn ? get(currentUser) : null;
      if (!owner) {
        // The refusal ended the session: what became of the account is not known here, so only the reason is said.
        toast(text, 'danger');
        if (alive) onclose();
        return;
      }
      if (owner.id !== presser?.id) {
        if (alive) onclose();
        return;
      }
      if (!alive) {
        toast(msg('profile.deleteAccountFailedFor', { name: owner.username, reason: text }), 'danger');
        return;
      }
      error = text;
      wrongPassword = isWrongPassword(err);
    } finally {
      if (alive) running = false;
    }
    await hand(() => document.getElementById(wrongPassword ? fieldId : failureId));
  }

  const heldReason = $derived(running ? msg('modals.actionRunning') : '');
  const needsPassword = $derived(!running && !password);
</script>

<Modal dismissible={!running} busy={running} open title={msg('profile.deleteAccountConfirmTitle')} width="44rem" initialFocus={`#${fieldId}`} {onclose}>
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void confirm();
    }}
  >
    <p class="text">{msg('profile.deleteAccountConfirmText')}</p>
    <TextField
      id={fieldId}
      label={msg('profile.deleteAccountPassword')}
      type="password"
      autocomplete="current-password"
      readonly={running}
      error={wrongPassword ? error : ''}
      bind:value={password}
    />
    {#if error && !wrongPassword}<Notice tone="danger" compact id={failureId} tabindex={-1}>{error}</Notice>{/if}
  </form>
  {#snippet footer()}
    <Button disabled={running} reason={heldReason} onclick={onclose}>{msg('common.cancel')}</Button>
    <Button variant="danger" disabled={needsPassword} reason={needsPassword ? msg('profile.deleteAccountPasswordNeeded') : ''} busy={running} onclick={confirm}>
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
</style>
