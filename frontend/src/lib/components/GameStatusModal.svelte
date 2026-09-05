<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { GAME_STATUSES, STATUS_LABELS, type GameStatus } from '../game/status';
  import { setStatus, type LibraryGame } from '../services/library';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let { open = $bindable(false), game }: { open?: boolean; game: LibraryGame } = $props();

  const current = $derived(game.status ?? '');

  const options = $derived<{ value: GameStatus; label: string }[]>([
    { value: '', label: msg('modals.gameStatusNone') },
    ...GAME_STATUSES.map((status) => ({ value: status, label: STATUS_LABELS[status] })),
  ]);

  async function pick(status: GameStatus) {
    try {
      await setStatus(game.id, status);
      open = false;
    } catch {
      toast(msg('modals.gameStatusUpdateFailed'), 'danger');
    }
  }
</script>

<Modal bind:open title={msg('modals.gameStatusTitle')}>
  <div class="options" role="group" aria-label={msg('modals.gameStatusTitle')}>
    {#each options as option (option.value)}
      <Button
        variant={option.value === current ? 'primary' : 'secondary'}
        pressed={option.value === current}
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
</style>
