<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { GAME_STATUSES, STATUS_LABELS, isFinished, type GameStatus } from '../game/status';
  import { COMPLETED_MIN, completionDateIssue, dateFromInput, dateInputValue, dayValue } from '../game/completion';
  import { libraryErrorText } from '../library/errors';
  import { setCompletedAt, setStatus, type LibraryGame } from '../services/library';
  import { focusHandOff } from '../utils/focusHandOff';
  import { errorCode, hasMessage, msg } from '../i18n';

  /**
   * The game's status, with the date it was finished for a finished one. A status without a date is
   * saved as it is pressed; a finished one asks for its date first. The date is a native date field:
   * the kit's TextField has no date type, and the native field brings the calendar and the locale's
   * order of day and month.
   */
  let {
    open = $bindable(false),
    game,
    returnFocus,
  }: {
    open?: boolean;
    game: LibraryGame;
    /** Where focus goes on close when the element that opened the dialog is gone (a menu item). */
    returnFocus?: () => HTMLElement | null | undefined;
  } = $props();
  const uid = $props.id();
  const dateId = `${uid}-date`;
  const dateErrorId = `${uid}-date-error`;
  const failureId = `${uid}-failure`;

  // The status this opening has saved, until the game's own copy follows: a failed date after it
  // leaves the dialog open on the saved status, and Save again sends only the date.
  let saved = $state<GameStatus | null>(null);
  const current = $derived(saved ?? game.status ?? '');

  const options = $derived<{ value: GameStatus; label: string }[]>([
    { value: '', label: msg('modals.gameStatusNone') },
    ...GAME_STATUSES.map((status) => ({ value: status, label: STATUS_LABELS[status] })),
  ]);

  let picked = $state<GameStatus | null>(null);
  let day = $state('');
  let saving = $state(false);
  // What the pressed button sends: a status, or the date's save. That button stays focused and busy.
  let sent = $state<GameStatus | 'save' | null>(null);
  // Why the status, or the date after a saved status, was refused, in the launcher's words.
  let failure = $state('');
  let dateFailure = $state('');

  const chosen = $derived(picked ?? (current as GameStatus));
  const dated = $derived(isFinished(chosen));
  const invalid = $derived(dated && completionDateIssue(day, new Date()));
  const dateError = $derived(invalid ? msg('modals.gameCompletedAtInvalid') : dateFailure);

  $effect(() => {
    if (open) {
      picked = null;
      saved = null;
      failure = '';
      dateFailure = '';
      day = untrack(() => dateInputValue(game.completedAt, new Date()));
    }
  });

  async function apply(status: GameStatus, from: GameStatus | 'save' = status) {
    if (saving) return;
    // The focus the pressed button holds goes to the words of a refusal: the date field for a date.
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    saving = true;
    sent = from;
    failure = '';
    dateFailure = '';
    let statusDone = false;
    try {
      if (status !== current) {
        await setStatus(game.id, status);
        saved = status;
      }
      statusDone = true;
      if (isFinished(status)) {
        const date = dateFromInput(day);
        if (date) await setCompletedAt(game.id, date);
      }
      open = false;
    } catch (err) {
      if (statusDone) dateFailure = dateFailureText(err);
      else failure = libraryErrorText(err, msg('modals.gameStatusUpdateFailed'));
    } finally {
      saving = false;
      sent = null;
    }
    if (dateFailure) await hand(() => document.getElementById(dateId));
    else if (failure) await hand(() => document.getElementById(failureId));
  }

  function dateFailureText(err: unknown): string {
    const code = errorCode(err);
    return hasMessage(code) ? msg('games.blocks.statusDateFailedReason', { reason: msg(code) }) : msg('games.blocks.statusDateFailed');
  }

  function pick(status: GameStatus) {
    if (saving) return;
    if (isFinished(status)) {
      picked = status;
      return;
    }
    void apply(status);
  }

  const heldReason = $derived(saving ? msg('modals.actionRunning') : '');
</script>

<Modal dismissible={!saving} bind:open title={msg('modals.gameStatusTitle')} initialFocus="[aria-pressed='true']" {returnFocus}>
  <div class="options" role="group" aria-label={msg('modals.gameStatusTitle')}>
    {#each options as option (option.value)}
      <Button
        variant={option.value === chosen ? 'primary' : 'secondary'}
        pressed={option.value === chosen}
        disabled={saving && sent !== option.value}
        reason={sent === option.value ? '' : heldReason}
        busy={saving && sent === option.value}
        onclick={() => pick(option.value)}
      >
        {option.label}
      </Button>
    {/each}
  </div>
  {#if failure}
    <div class="notice">
      <Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice>
    </div>
  {/if}
  {#if dated}
    <div class="date">
      <label for={dateId}>{msg('modals.gameCompletedAt')}</label>
      <input
        id={dateId}
        class="input"
        class:invalid={!!dateError}
        type="date"
        bind:value={day}
        oninput={() => (dateFailure = '')}
        min={COMPLETED_MIN}
        max={dayValue(new Date())}
        required
        readonly={saving}
        aria-invalid={dateError ? 'true' : undefined}
        aria-describedby={dateError ? dateErrorId : undefined}
      />
      {#if dateError}
        {#key dateError}
          <p class="error" id={dateErrorId} role="alert">{dateError}</p>
        {/key}
      {/if}
    </div>
    <div class="actions">
      <Button
        variant="primary"
        disabled={(saving && sent !== 'save') || invalid}
        reason={invalid ? msg('modals.gameCompletedAtInvalid') : sent === 'save' ? '' : heldReason}
        busy={sent === 'save'}
        onclick={() => apply(chosen, 'save')}
      >
        {msg('common.save')}
      </Button>
    </div>
  {/if}
</Modal>

<style>
  .options {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .notice {
    margin-top: var(--space-3);
  }

  .date {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin-top: var(--space-4);
  }

  label {
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .date .input {
    color-scheme: dark light;
  }

  .date .input.invalid {
    border-color: var(--danger);
  }

  /* As TextField's error: on a plate of its own, so the danger colour keeps its contrast. */
  .error {
    align-self: flex-start;
    margin: 0;
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
    background: var(--surface-2);
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: var(--space-4);
  }
</style>
