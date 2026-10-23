<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { GAME_STATUSES, STATUS_LABELS, isFinished, type GameStatus } from '../game/status';
  import { COMPLETED_MIN, completionDateIssue, dateFromInput, dateInputValue, dayValue } from '../game/completion';
  import { libraryErrorText } from '../library/errors';
  import { setCompletedAt, setStatus, type LibraryGame } from '../services/library';
  import { toast } from '../stores/toasts';
  import { msg } from '../i18n';

  let { open = $bindable(false), game }: { open?: boolean; game: LibraryGame } = $props();

  const current = $derived(game.status ?? '');

  const options = $derived<{ value: GameStatus; label: string }[]>([
    { value: '', label: msg('modals.gameStatusNone') },
    ...GAME_STATUSES.map((status) => ({ value: status, label: STATUS_LABELS[status] })),
  ]);

  let picked = $state<GameStatus | null>(null);
  let day = $state('');
  let saving = $state(false);

  const today = $derived(dayValue(new Date()));
  const chosen = $derived(picked ?? (current as GameStatus));
  const dated = $derived(isFinished(chosen));
  const invalid = $derived(dated && completionDateIssue(day, new Date()));

  $effect(() => {
    if (open) {
      picked = null;
      day = untrack(() => dateInputValue(game.completedAt, new Date()));
    }
  });

  async function apply(status: GameStatus) {
    saving = true;
    try {
      if (status !== current) await setStatus(game.id, status);
      if (isFinished(status)) {
        const date = dateFromInput(day);
        if (date) await setCompletedAt(game.id, date);
      }
      open = false;
    } catch (err) {
      toast(libraryErrorText(err, msg('modals.gameStatusUpdateFailed')), 'danger');
    } finally {
      saving = false;
    }
  }

  function pick(status: GameStatus) {
    if (isFinished(status)) {
      picked = status;
      return;
    }
    void apply(status);
  }
</script>

<Modal bind:open title={msg('modals.gameStatusTitle')}>
  <div class="options" role="group" aria-label={msg('modals.gameStatusTitle')}>
    {#each options as option (option.value)}
      <Button
        variant={option.value === chosen ? 'primary' : 'secondary'}
        pressed={option.value === chosen}
        disabled={saving}
        onclick={() => pick(option.value)}
      >
        {option.label}
      </Button>
    {/each}
  </div>
  {#if dated}
    <label class="date">
      <span>{msg('modals.gameCompletedAt')}</span>
      <input type="date" bind:value={day} min={COMPLETED_MIN} max={today} required aria-invalid={invalid} />
    </label>
    <div class="actions">
      <Button variant="primary" disabled={saving || invalid} onclick={() => apply(chosen)}>{msg('common.save')}</Button>
    </div>
  {/if}
</Modal>

<style>
  .options {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .date {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin-top: var(--space-4);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .date input {
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text);
    font: inherit;
    color-scheme: dark light;
  }

  .date input[aria-invalid='true'] {
    border-color: var(--danger);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: var(--space-4);
  }
</style>
