<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { X } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import Select from '../../lib/components/Select.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import TextArea from '../../lib/components/TextArea.svelte';
  import TextField from '../../lib/components/TextField.svelte';
  import GamePicker from '../../lib/components/profile/GamePicker.svelte';
  import { msg } from '../../lib/i18n';
  import { accountErrorField } from '../../lib/services/accountMessages';
  import { onLogUploadStatus, type LogUploadStatus } from '../../lib/services/logsUpload';
  import { ticketLimits, type Limits, type SupportTopic } from '../../lib/services/support';
  import { supportErrorText } from '../../lib/services/supportMessages';
  import {
    announceCreated,
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
  import { bodyCounter, isPlainEnter, isSendShortcut, sendKeys } from './sending';

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
  let subjectField = $state<HTMLInputElement>();
  let bodyField = $state<HTMLTextAreaElement>();

  function sendOn(match: (event: KeyboardEvent) => boolean) {
    return (event: KeyboardEvent) => {
      if (!match(event)) return;
      event.preventDefault();
      void submit();
    };
  }

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

  // A refusal about the subject or the text is said at that field, which takes focus; any other once,
  // in the form's notice.
  async function showError(err: unknown) {
    const field = accountErrorField(err);
    const text = supportErrorText(err, msg('errSupport.generic'));
    if (field === 'subject') errors = { ...errors, subject: text };
    else if (field === 'body') errors = { ...errors, body: text };
    else formError = text;
    await focusInvalid();
  }

  async function focusInvalid() {
    await tick();
    if (errors.subject) subjectField?.focus();
    else if (errors.body) bodyField?.focus();
  }

  async function submit() {
    if (sending || !limits || !isTopic(topic)) return;
    errors = validateDraft({ subject, body }, limits);
    formError = '';
    if (hasErrors(errors)) {
      await focusInvalid();
      return;
    }
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
      sending = false;
      await showError(err);
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

    <!-- While the request is on its way the fields refuse edits by being read-only: a field that turns
         disabled drops the focus a send shortcut was pressed in. TextField has no counter, so the
         subject's count is its hint. -->
    <TextField
      id="{uid}-subject"
      label={msg('tickets.subjectLabel')}
      autocomplete="off"
      hint={msg('tickets.counter', { count: runeCount(subject.trim()), max: limits.maxSubjectRunes })}
      readonly={sending}
      error={errors.subject}
      bind:value={subject}
      bind:input={subjectField}
      oninput={() => (errors = { ...errors, subject: undefined })}
      onkeydown={sendOn(isPlainEnter)}
    />

    <TextArea
      id="{uid}-body"
      label={msg('tickets.bodyLabel')}
      rows={8}
      grow={16}
      placeholder={msg('tickets.bodyPlaceholder')}
      counter={bodyCounter(limits.maxBodyRunes)}
      hint={msg('tickets.sendHint', { keys: sendKeys() })}
      readonly={sending}
      error={errors.body}
      bind:value={body}
      bind:textarea={bodyField}
      oninput={() => (errors = { ...errors, body: undefined })}
      onkeydown={sendOn(isSendShortcut)}
    />

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
