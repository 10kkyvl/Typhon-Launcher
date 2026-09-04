<script lang="ts">
  import { onDestroy } from 'svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { profile, profileByCode, sendRequest, type PublicProfile } from '../../lib/services/social';
  import { commonLine, isFriendCode, relationHint } from '../../lib/social/view';
  import { toast } from '../../lib/stores/toasts';

  let { open = $bindable(false), onsent }: { open?: boolean; onsent?: () => void } = $props();

  let query = $state('');
  let found = $state<PublicProfile | null>(null);
  let error = $state('');
  let searching = $state(false);
  let sending = $state(false);

  let timer: ReturnType<typeof setTimeout> | undefined;
  let attempt = 0;

  const canSend = $derived(!!found && found.relation === 'none');
  const mutual = $derived(found ? commonLine(found.mutualCount, found.common?.count ?? 0) : '');
  const hint = $derived(found && !canSend ? relationHint(found.relation) : '');

  function reset() {
    query = '';
    found = null;
    error = '';
    searching = false;
    attempt += 1;
    clearTimeout(timer);
  }

  async function lookup(input: string) {
    const term = input.trim();
    attempt += 1;
    const current = attempt;
    if (!term || term === '@') {
      found = null;
      error = '';
      searching = false;
      return;
    }
    searching = true;
    try {
      const result = isFriendCode(term)
        ? await profileByCode(term)
        : await profile(term.replace(/^@/, ''));
      if (current !== attempt) return;
      found = result;
      error = '';
    } catch (err) {
      if (current !== attempt) return;
      found = null;
      error = accountErrorText(err, 'Не удалось найти пользователя');
    } finally {
      if (current === attempt) searching = false;
    }
  }

  function schedule(value: string) {
    clearTimeout(timer);
    timer = setTimeout(() => lookup(value), 300);
  }

  function submit(event: KeyboardEvent) {
    if (event.key !== 'Enter') return;
    clearTimeout(timer);
    lookup(query);
  }

  onDestroy(() => clearTimeout(timer));

  async function send() {
    if (!found || !canSend || sending) return;
    sending = true;
    try {
      const result = await sendRequest(found.username);
      toast(result.accepted ? 'Вы теперь друзья' : 'Заявка отправлена', 'success');
      open = false;
      reset();
      onsent?.();
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось отправить заявку'), 'danger');
    } finally {
      sending = false;
    }
  }
</script>

<Modal bind:open title="Добавить друга" width="52rem" onclose={reset}>
  <div class="body">
    <SearchInput
      bind:value={query}
      placeholder="@имя или код TY-XXXX-XXXX"
      loading={searching}
      oninput={schedule}
      onkeydown={submit}
    />

    {#if found}
      <div class="preview">
        <Avatar size="md" name={found.displayName || found.username} src={found.avatarUrl} />
        <div class="info">
          <span class="name">{found.displayName || found.username}</span>
          <span class="handle">@{found.username}</span>
          {#if found.bio}
            <p class="bio">{found.bio}</p>
          {/if}
          {#if mutual}
            <span class="mutual">{mutual}</span>
          {/if}
        </div>
      </div>
      {#if hint}
        <p class="hint">{hint}</p>
      {/if}
    {:else if error}
      <p class="hint danger">{error}</p>
    {:else if !query.trim()}
      <p class="hint">Найдите по имени пользователя или по коду, которым с вами поделились.</p>
    {/if}
  </div>

  {#snippet footer()}
    <Button
      disabled={sending}
      onclick={() => {
        open = false;
        reset();
      }}
    >
      Отмена
    </Button>
    {#if canSend}
      <Button variant="primary" disabled={sending} onclick={send}>Отправить заявку</Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .preview {
    display: flex;
    align-items: flex-start;
    gap: var(--space-4);
    padding: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--surface-2);
  }

  .info {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .name {
    font-size: var(--font-lg);
    font-weight: 600;
    line-height: 1.3;
  }

  .handle {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .bio {
    margin-top: 0.4rem;
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text-2);
  }

  .mutual {
    margin-top: 0.4rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .hint {
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text-3);
  }

  .hint.danger {
    color: var(--danger);
  }
</style>
