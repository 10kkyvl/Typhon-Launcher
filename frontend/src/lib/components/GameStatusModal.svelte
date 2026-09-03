<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { GAME_STATUSES, STATUS_LABELS, type GameStatus } from '../game/status';
  import { setStatus, type LibraryGame } from '../services/library';
  import { toast } from '../stores/toasts';

  let { open = $bindable(false), game }: { open?: boolean; game: LibraryGame } = $props();

  const options: { value: GameStatus; label: string }[] = [
    { value: '', label: 'Без статуса' },
    ...GAME_STATUSES.map((status) => ({ value: status, label: STATUS_LABELS[status] })),
  ];

  async function pick(status: GameStatus) {
    try {
      await setStatus(game.id, status);
      open = false;
    } catch {
      toast('Не удалось изменить статус', 'danger');
    }
  }
</script>

<Modal bind:open title="Статус игры">
  <div class="options">
    {#each options as option (option.value)}
      <Button
        variant={(game.status ?? '') === option.value ? 'primary' : 'secondary'}
        onclick={() => pick(option.value)}
      >
        {option.label}
      </Button>
    {/each}
  </div>
</Modal>

<style>
  .options {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .options :global(.btn) {
    width: 100%;
  }
</style>
