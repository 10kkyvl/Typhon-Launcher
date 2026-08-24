<script lang="ts">
  import { acceptSourcesNotice } from '../stores/sourcesNotice';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  let {
    open = $bindable(false),
    mode = 'gate',
    onaccepted,
  }: {
    open?: boolean;
    mode?: 'gate' | 'review';
    onaccepted?: () => void;
  } = $props();

  let saving = $state(false);

  async function confirm() {
    saving = true;
    try {
      const accepted = await acceptSourcesNotice();
      if (!accepted) return;
      open = false;
      onaccepted?.();
    } finally {
      saving = false;
    }
  }

  function cancel() {
    open = false;
  }
</script>

<Modal bind:open title="Прежде чем добавить источник" width="48rem">
  <p class="notice-text">
    Typhon не предоставляет и не проверяет содержимое сторонних источников. Добавляйте только источники и материалы,
    которыми вы имеете право пользоваться. Адрес источника и его содержимое обрабатываются на этом устройстве —
    Typhon не передаёт их своим серверам.
  </p>
  {#snippet footer()}
    {#if mode === 'review'}
      <Button onclick={cancel}>Закрыть</Button>
    {:else}
      <Button onclick={cancel}>Отмена</Button>
      <Button variant="primary" disabled={saving} onclick={confirm}>
        {saving ? 'Сохранение…' : 'Понятно, продолжить'}
      </Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .notice-text {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
    max-width: var(--prose-max);
  }
</style>
