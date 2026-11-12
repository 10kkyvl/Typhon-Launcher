<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { X } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import Select from '../../lib/components/Select.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
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

  let { initialTopic, initialSubject = '' }: { initialTopic: SupportTopic; initialSubject?: string } = $props();
  const uid = $props.id();

  let topic = $state<string>(untrack(() => initialTopic));
  let attachLogs = $state(untrack(() => attachLogsByDefault(initialTopic)));
  let logsTouched = false;
  let game = $state<{ igdbId: number; title: string } | null>(null);
  let pickerOpen = $state(false);
  let subject = $state(untrack(() => initialSubject));
  let body = $state('');
  let limits = $state<Limits | null>(null);
  let limitsFailure = $state('');
  let errors = $state<DraftErrors>({});
  let formError = $state('');
  let sending = $state(false);
  let limitsRetrying = $state(false);
  let upload = $state<LogUploadStatus | null>(null);
  let confirm: LogsConfirm;

  const topics = $derived(topicOptions());
  const stage = $derived(logsStageText(upload));

  let form = $state<HTMLElement>();

  // A failure stays on screen until an answer replaces it.
  async function loadLimits(): Promise<boolean> {
    try {
      limits = await ticketLimits();
      limitsFailure = '';
      return true;
    } catch (err) {
      limitsFailure = supportErrorText(err, msg('tickets.limitsFailed'));
      return false;
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
    topic = id;
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

<PageHeader title={msg('tickets.formTitle')} subtitle={msg('tickets.subtitle')} />

{#if limitsFailure || limitsRetrying}
  <Notice tone="danger" bind:retrying={limitsRetrying} retry={{ label: msg('common.retry'), run: loadLimits, within: () => form }}>
    {limitsFailure}
  </Notice>
{:else if !limits}
  <Spinner label={msg('tickets.loading')} />
{:else}
  <div class="form" bind:this={form}>
    <div class="field">
      <span class="label" id="{uid}-topic-label">{msg('tickets.topicLabel')}</span>
      <Select options={topics} value={topic} width="100%" labelledby="{uid}-topic-label" onchange={changeTopic} />
    </div>

    <div class="field">
      <span class="label">{msg('tickets.gameLabel')}</span>
      {#if game}
        <div class="chosen">
          <span class="chosen-title" dir="auto">{game.title}</span>
          <IconButton size="sm" label={msg('tickets.gameClear')} disabled={sending} onclick={() => (game = null)}>
            <X size="1.5rem" strokeWidth={1.8} />
          </IconButton>
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

    <div class="field">
      <label class="label" for="{uid}-subject">
        {msg('tickets.subjectLabel')}
        <span class="counter" class:over={runeCount(subject.trim()) > limits.maxSubjectRunes}>
          {msg('tickets.counter', { count: runeCount(subject.trim()), max: limits.maxSubjectRunes })}
        </span>
      </label>
      <input
        id="{uid}-subject"
        class="input"
        class:invalid={!!errors.subject}
        aria-invalid={errors.subject ? 'true' : undefined}
        aria-describedby={errors.subject ? `${uid}-subject-error` : undefined}
        type="text"
        dir="auto"
        autocomplete="off"
        bind:value={subject}
        disabled={sending}
        oninput={() => (errors = { ...errors, subject: undefined })}
      />
      {#if errors.subject}<span class="error" id="{uid}-subject-error" role="alert">{errors.subject}</span>{/if}
    </div>

    <div class="field">
      <label class="label" for="{uid}-body">
        {msg('tickets.bodyLabel')}
        <span class="counter" class:over={bodyLength(body) > limits.maxBodyRunes}>
          {msg('tickets.counter', { count: bodyLength(body), max: limits.maxBodyRunes })}
        </span>
      </label>
      <textarea
        id="{uid}-body"
        class="input area"
        class:invalid={!!errors.body}
        aria-invalid={errors.body ? 'true' : undefined}
        aria-describedby={errors.body ? `${uid}-body-error` : undefined}
        rows="8"
        dir="auto"
        placeholder={msg('tickets.bodyPlaceholder')}
        bind:value={body}
        disabled={sending}
        oninput={() => (errors = { ...errors, body: undefined })}
      ></textarea>
      {#if errors.body}<span class="error" id="{uid}-body-error" role="alert">{errors.body}</span>{/if}
    </div>

    <AttachLogs
      bind:checked={attachLogs}
      disabled={sending}
      onchange={() => (logsTouched = true)}
    />

    {#if formError}
      <Notice tone="danger">{formError}</Notice>
    {/if}

    {#if sending && stage}
      <div class="progress">
        <span class="hint">{stage}</span>
        <ProgressBar
          value={upload ? uploadPercent(upload) : 0}
          indeterminate={upload?.state !== 'sending' || !upload.totalBytes}
          height={5}
          label={msg('settings.aboutLogsLabel')}
        />
      </div>
    {/if}

    <div class="actions">
      <Button disabled={sending} onclick={goBack}>{msg('common.cancel')}</Button>
      <Button variant="primary" busy={sending} onclick={submit}>
        {sending ? msg('tickets.submitting') : msg('tickets.submit')}
      </Button>
    </div>
  </div>
{/if}

<LogsConfirm bind:this={confirm} />

<style>
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
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  /* A field message keeps its place next to the field, on a plate of its own: the page behind it is
     the window backdrop. */
  .counter.over {
    background: var(--surface-2);
    color: var(--danger);
  }

  .input.invalid {
    border-color: var(--danger);
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

  .picker-toggle {
    display: flex;
  }

  .error {
    align-self: flex-start;
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
    background: var(--surface-2);
    font-size: var(--font-xs);
    color: var(--danger);
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
