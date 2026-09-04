<script lang="ts">
  import { onMount } from 'svelte';
  import { Copy } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { friendCode, rotateFriendCode } from '../../lib/services/social';
  import { toast } from '../../lib/stores/toasts';

  let { code = $bindable('') }: { code?: string } = $props();

  let loading = $state(!code);
  let rotating = $state(false);
  let confirmOpen = $state(false);

  async function load() {
    loading = true;
    try {
      code = await friendCode();
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось получить код'), 'danger');
    } finally {
      loading = false;
    }
  }

  async function copy() {
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code);
      toast('Скопировано', 'info');
    } catch {
      toast('Не удалось скопировать', 'danger');
    }
  }

  async function rotate() {
    if (rotating) return;
    rotating = true;
    try {
      code = await rotateFriendCode();
      confirmOpen = false;
      toast('Новый код готов', 'success');
    } catch (err) {
      toast(accountErrorText(err, 'Не удалось сменить код'), 'danger');
    } finally {
      rotating = false;
    }
  }

  onMount(() => {
    if (!code) load();
  });
</script>

<Card padding="var(--space-4) var(--space-5)">
  <div class="card">
    <div class="text">
      <span class="label">Ваш код</span>
      <span class="code" class:pending={loading}>{loading ? 'Загрузка…' : code || '—'}</span>
    </div>
    <div class="actions">
      <IconButton label="Скопировать код" size="sm" disabled={!code} onclick={copy}>
        <Copy size="1.6rem" strokeWidth={1.8} />
      </IconButton>
      <Button variant="ghost" size="sm" disabled={!code || rotating} onclick={() => (confirmOpen = true)}>
        Сгенерировать новый
      </Button>
    </div>
  </div>
</Card>

<Modal bind:open={confirmOpen} title="Сменить код" width="42rem">
  <p class="warn">
    Старый код перестанет работать: по нему вас больше никто не найдёт. Уже отправленные заявки и список
    друзей это не затронет.
  </p>
  {#snippet footer()}
    <Button disabled={rotating} onclick={() => (confirmOpen = false)}>Отмена</Button>
    <Button variant="danger" disabled={rotating} onclick={rotate}>
      {rotating ? 'Смена…' : 'Сгенерировать новый'}
    </Button>
  {/snippet}
</Modal>

<style>
  .card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-5);
    flex-wrap: wrap;
  }

  .text {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .code {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: 0.12em;
    font-variant-numeric: tabular-nums;
  }

  .code.pending {
    font-size: var(--font-md);
    font-weight: 500;
    letter-spacing: normal;
    color: var(--text-3);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .warn {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
  }
</style>
