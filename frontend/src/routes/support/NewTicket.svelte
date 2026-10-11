<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { CircleAlert, X } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import Select from '../../lib/components/Select.svelte';
  import GamePicker from '../../lib/components/profile/GamePicker.svelte';
  import { msg } from '../../lib/i18n';
  import { accountErrorField } from '../../lib/services/accountMessages';
  import { onLogUploadStatus, type LogUploadStatus } from '../../lib/services/logsUpload';
  import { ticketLimits, type Limits, type SupportTopic } from '../../lib/services/support';
  import { supportErrorText } from '../../lib/services/supportMessages';
  import {
    announceCreated,
    bodyLength,
    hasErrors,
    sendNewTicket,
    validateDraft,
    type DraftErrors,
  } from '../../lib/support/draft';
  import { logsStageText, uploadPercent } from '../../lib/support/progress';
  import { attachLogsByDefault, isTopic, topicOptions } from '../../lib/support/topics';
  import { goBack, navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { runeCount } from '../../lib/profile/layout';
  import AttachLogs from './AttachLogs.svelte';
  import LogsConfirm from './LogsConfirm.svelte';

  let { initialTopic }: { initialTopic: SupportTopic } = $props();

  let topic = $state<string>(untrack(() => initialTopic));
  let attachLogs = $state(untrack(() => attachLogsByDefault(initialTopic)));
  let logsTouched = false;
  let game = $state<{ igdbId: number; title: string } | null>(null);
  let pickerOpen = $state(false);
  let subject = $state('');
  let body = $state('');
  let limits = $state<Limits | null>(null);
  let limitsFailure = $state('');
  let errors = $state<DraftErrors>({});
  let formError = $state('');
  let sending = $state(false);
  let upload = $state<LogUploadStatus | null>(null);
  let confirm: LogsConfirm;

  const topics = $derived(topicOptions());
  const stage = $derived(logsStageText(upload));

  async function loadLimits() {
    limitsFailure = '';
    try {
      limits = await ticketLimits();
    } catch (err) {
      limitsFailure = supportErrorText(err, msg('tickets.limitsFailed'));
    }
  }

  let onScreen = true;

  onMount(() => {
    void loadLimits();
    const off = onLogUploadStatus((status) => {
      if (sending) upload = status;
    });
    return () => {
      onScreen = false;
      off();
    };
  });

  function changeTopic(id: string) {
    if (!logsTouched && isTopic(id)) attachLogs = attachLogsByDefault(id);
  }

  function pickGame(pick: { igdbId: number; game: { title: string } }) {
    game = { igdbId: pick.igdbId, title: pick.game.title };
    pickerOpen = false;
  }

  function showError(err: unknown) {
    const field = accountErrorField(err);
    const text = supportErrorText(err, msg('errSupport.generic'));
    if (field === 'subject') errors = { ...errors, subject: text };
    else if (field === 'body') errors = { ...errors, body: text };
    else formError = text;
  }

  async function submit() {
    if (sending || !limits || !isTopic(topic)) return;
    errors = validateDraft({ subject, body }, limits);
    formError = '';
    if (hasErrors(errors)) return;
    sending = true;
    upload = null;
    try {
      const outcome = await sendNewTicket(
        { topic, subject, body, gameIgdbId: game?.igdbId ?? null, attachLogs },
        () => confirm.ask(),
      );
      if (!outcome.sent) return;
      announceCreated(outcome.value, onScreen, {
        notify: (text) => toast(text, 'success'),
        open: (number) => navigate('support', { ticket: String(number) }),
      });
    } catch (err) {
      showError(err);
    } finally {
      sending = false;
      upload = null;
    }
  }
</script>

<Card surface="panel">
  <PageHeader title={msg('tickets.formTitle')} subtitle={msg('tickets.subtitle')} />

  {#if limitsFailure}
    <div class="banner" role="alert">
      <CircleAlert size="1.6rem" strokeWidth={1.8} />
      <span class="banner-text">{limitsFailure}</span>
      <Button size="sm" onclick={loadLimits}>{msg('common.retry')}</Button>
    </div>
  {:else if !limits}
    <p class="muted">{msg('tickets.loading')}</p>
  {:else}
    <div class="form">
      <div class="field">
        <span class="label">{msg('tickets.topicLabel')}</span>
        <Select options={topics} bind:value={topic} width="100%" onchange={changeTopic} />
      </div>

      <div class="field">
        <span class="label">{msg('tickets.gameLabel')}</span>
        {#if game}
          <div class="chosen">
            <span class="chosen-title" dir="auto">{game.title}</span>
            <button type="button" class="clear" aria-label={msg('tickets.gameClear')} disabled={sending} onclick={() => (game = null)}>
              <X size="1.5rem" strokeWidth={1.8} />
            </button>
          </div>
        {:else}
          <p class="hint">{msg('tickets.gameNone')}</p>
        {/if}
        <div class="picker-toggle">
          <Button size="sm" disabled={sending} onclick={() => (pickerOpen = !pickerOpen)}>
            {pickerOpen ? msg('tickets.gameCollapse') : msg('tickets.gameChoose')}
          </Button>
        </div>
        {#if pickerOpen}
          <GamePicker chosen={game ? [game.igdbId] : []} disabled={sending} onpick={pickGame} />
        {/if}
      </div>

      <label class="field">
        <span class="label">
          {msg('tickets.subjectLabel')}
          <span class="counter" class:over={runeCount(subject.trim()) > limits.maxSubjectRunes}>
            {msg('tickets.counter', { count: runeCount(subject.trim()), max: limits.maxSubjectRunes })}
          </span>
        </span>
        <input
          class="input"
          class:invalid={!!errors.subject}
          type="text"
          dir="auto"
          autocomplete="off"
          bind:value={subject}
          disabled={sending}
          oninput={() => (errors = { ...errors, subject: undefined })}
        />
        {#if errors.subject}<span class="error" role="alert">{errors.subject}</span>{/if}
      </label>

      <label class="field">
        <span class="label">
          {msg('tickets.bodyLabel')}
          <span class="counter" class:over={bodyLength(body) > limits.maxBodyRunes}>
            {msg('tickets.counter', { count: bodyLength(body), max: limits.maxBodyRunes })}
          </span>
        </span>
        <textarea
          class="input area"
          class:invalid={!!errors.body}
          rows="8"
          dir="auto"
          placeholder={msg('tickets.bodyPlaceholder')}
          bind:value={body}
          disabled={sending}
          oninput={() => (errors = { ...errors, body: undefined })}
        ></textarea>
        {#if errors.body}<span class="error" role="alert">{errors.body}</span>{/if}
      </label>

      <AttachLogs
        bind:checked={attachLogs}
        disabled={sending}
        onchange={() => (logsTouched = true)}
      />

      {#if formError}
        <p class="error general" role="alert">
          <CircleAlert size="1.6rem" strokeWidth={1.8} />
          <span>{formError}</span>
        </p>
      {/if}

      {#if sending && stage}
        <div class="progress">
          <span class="hint">{stage}</span>
          <ProgressBar
            value={upload ? uploadPercent(upload) : 0}
            indeterminate={upload?.state !== 'sending' || !upload.totalBytes}
            height={5}
          />
        </div>
      {/if}

      <div class="actions">
        <Button disabled={sending} onclick={goBack}>{msg('common.cancel')}</Button>
        <Button variant="primary" disabled={sending} onclick={submit}>
          {sending ? msg('tickets.submitting') : msg('tickets.submit')}
        </Button>
      </div>
    </div>
  {/if}
</Card>

<LogsConfirm bind:this={confirm} />

<style>
  .muted {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .banner {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 0.9rem 1.2rem;
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
    color: var(--danger);
    font-size: var(--font-sm);
  }

  .banner-text {
    flex: 1;
    min-width: 0;
    line-height: 1.4;
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    max-width: 72rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
  }

  .label {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.8rem;
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .counter {
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .counter.over {
    color: var(--danger);
  }

  .input {
    width: 100%;
    height: var(--control-md);
    padding: 0 1.2rem;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    color: var(--text);
    font-size: var(--font-sm);
    font-family: inherit;
    transition:
      border-color var(--dur) var(--ease),
      box-shadow var(--dur) var(--ease);
  }

  .input:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-subtle);
  }

  .input.invalid {
    border-color: var(--danger);
  }

  .input:disabled {
    opacity: 0.6;
  }

  .area {
    height: auto;
    padding: 0.9rem 1.2rem;
    line-height: 1.55;
    resize: vertical;
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.45;
    color: var(--text-3);
  }

  .chosen {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: 0.6rem 0.6rem 0.6rem 1.2rem;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    font-size: var(--font-sm);
  }

  .chosen-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .clear {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.8rem;
    height: 2.8rem;
    border-radius: var(--radius-sm);
    color: var(--text-3);
  }

  .clear:hover:not(:disabled) {
    background: var(--hover-strong);
    color: var(--text);
  }

  .picker-toggle {
    display: flex;
  }

  .error {
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .error.general {
    display: flex;
    align-items: flex-start;
    gap: 0.8rem;
    margin: 0;
    padding: 0.9rem 1.2rem;
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
    font-size: var(--font-sm);
    line-height: 1.4;
  }

  .error.general :global(svg) {
    flex-shrink: 0;
    margin-top: 0.1rem;
  }

  .progress {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

</style>
