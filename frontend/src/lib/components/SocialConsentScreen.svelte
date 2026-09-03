<script lang="ts">
  import { updateSettings } from '../stores/settings';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  let { open = $bindable(false), onclose }: { open?: boolean; onclose?: () => void } = $props();

  let saving = $state(false);

  function close() {
    open = false;
    onclose?.();
  }

  async function enable() {
    if (saving) return;
    saving = true;
    try {
      await updateSettings({ accountSync: true });
      close();
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:open title="Друзья и синхронизация" width="50rem" onclose={() => onclose?.()}>
  <div class="body">
    <p class="text">
      Друзья работают поверх синхронизации с аккаунтом: без неё серверу нечего показать вашим друзьям, а вам —
      их профили.
    </p>
    <p class="text">С устройства уходит только то, что вы видите в профиле:</p>
    <ul class="list">
      <li>список игр в библиотеке;</li>
      <li>наигранное время;</li>
      <li>отметки «любимая» и статусы прохождения;</li>
      <li>даты, когда эти отметки и статусы поставлены.</li>
    </ul>
    <p class="text">
      Файлы игр, источники, загрузки и что-либо ещё с диска не отправляются. Кто увидит эти данные, задаётся в
      настройках профиля, а синхронизацию можно выключить в любой момент.
    </p>
  </div>

  {#snippet footer()}
    <Button disabled={saving} onclick={close}>Не сейчас</Button>
    <Button variant="primary" disabled={saving} onclick={enable}>
      {saving ? 'Включение…' : 'Включить синхронизацию'}
    </Button>
  {/snippet}
</Modal>

<style>
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .text {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding-left: 2rem;
    list-style: disc;
    font-size: var(--font-sm);
    line-height: 1.55;
    color: var(--text-2);
  }
</style>
