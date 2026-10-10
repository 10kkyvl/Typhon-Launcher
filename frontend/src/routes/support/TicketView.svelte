<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { ArrowLeft, CircleAlert, Paperclip } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import type { ConfirmPrompt } from '../../lib/confirm/prompts';
  import { msg } from '../../lib/i18n';
  import { onLogUploadStatus, type LogUploadStatus } from '../../lib/services/logsUpload';
  import {
    getTicket,
    resolveTicket,
    ticketLimits,
    type Limits,
    type TicketThread,
  } from '../../lib/services/support';
  import { supportErrorText } from '../../lib/services/supportMessages';
  import { bodyLength, sendReply, validateBody } from '../../lib/support/draft';
  import { unreadGrew } from '../../lib/support/unread';
  import { logsStageText, uploadPercent } from '../../lib/support/progress';
  import { statusView, topicLabel } from '../../lib/support/topics';
  import { navigate } from '../../lib/stores/router';
  import { refreshUnread, unreadReplies } from '../../lib/stores/support';
  import { toast } from '../../lib/stores/toasts';
  import { dateTime } from '../../lib/utils/format';
  import AttachLogs from './AttachLogs.svelte';
  import LogsConfirm from './LogsConfirm.svelte';

  let { number }: { number: number } = $props();

  let thread = $state<TicketThread | null>(null);
  let loading = $state(true);
  let failure = $state('');
  let limits = $state<Limits | null>(null);
  let limitsFailure = $state('');
  let reply = $state('');
  let replyError = $state('');
  let attachLogs = $state(false);
  let sending = $state(false);
  let upload = $state<LogUploadStatus | null>(null);
  let resolving = $state<ConfirmPrompt | null>(null);
  let confirm: LogsConfirm;
  let run = 0;

  const ticket = $derived(thread?.ticket ?? null);
  const status = $derived(ticket ? statusView(ticket.status) : null);
  const resolved = $derived(ticket?.status === 'resolved');
  const stage = $derived(logsStageText(upload));

  async function load() {
    const current = ++run;
    failure = '';
    try {
      const next = await getTicket(number);
      if (current !== run) return;
      thread = next;
      void refreshUnread();
    } catch (err) {
      if (current !== run) return;
      failure = supportErrorText(err, msg('tickets.ticketLoadFailed'));
    } finally {
      if (current === run) loading = false;
    }
  }

  async function loadLimits() {
    limitsFailure = '';
    try {
      limits = await ticketLimits();
    } catch (err) {
      limitsFailure = supportErrorText(err, msg('tickets.limitsFailed'));
    }
  }

  function retry() {
    loading = thread === null;
    void load();
  }

  onMount(() => {
    void load();
    void loadLimits();
    const off = onLogUploadStatus((next) => {
      if (sending) upload = next;
    });
    return () => {
      run += 1;
      off();
    };
  });

  let lastUnread: number | null = null;
  $effect(() => {
    const count = $unreadReplies;
    untrack(() => {
      const grew = unreadGrew(lastUnread, count);
      lastUnread = count;
      if (grew && thread && !sending) void load();
    });
  });

  function when(iso: string): string {
    const value = new Date(iso);
    return Number.isNaN(value.getTime()) ? '' : dateTime(value);
  }

  async function submitReply() {
    if (sending || !limits) return;
    replyError = validateBody(reply, limits);
    if (replyError) return;
    sending = true;
    upload = null;
    try {
      const outcome = await sendReply(number, reply, attachLogs, () => confirm.ask());
      if (!outcome.sent) return;
      reply = '';
      attachLogs = false;
      toast(msg('tickets.replySent'), 'success');
      await load();
    } catch (err) {
      replyError = supportErrorText(err, msg('tickets.replyFailed'));
    } finally {
      sending = false;
      upload = null;
    }
  }

  function askResolve() {
    if (!ticket) return;
    resolving = {
      title: msg('tickets.resolveTitle'),
      text: msg('tickets.resolveText', { ref: ticket.ref }),
      confirm: msg('tickets.resolve'),
      busy: msg('tickets.resolveBusy'),
    };
  }

  async function confirmResolve() {
    try {
      const updated = await resolveTicket(number);
      if (thread) thread = { ...thread, ticket: updated };
      toast(msg('tickets.resolved'), 'success');
    } catch (err) {
      toast(supportErrorText(err, msg('tickets.resolveFailed')), 'danger');
    }
  }
</script>

<Card surface="panel">
  <div class="top">
    <Button variant="ghost" size="sm" onclick={() => navigate('support')}>
      <ArrowLeft size="1.5rem" strokeWidth={1.8} />
      {msg('tickets.back')}
    </Button>
  </div>

  {#if failure}
    <div class="banner" role="alert">
      <CircleAlert size="1.6rem" strokeWidth={1.8} />
      <span class="banner-text">{failure}</span>
      <Button size="sm" onclick={retry}>{msg('common.retry')}</Button>
    </div>
  {/if}

  {#if loading}
    <p class="muted">{msg('tickets.loading')}</p>
  {:else if ticket && thread}
    <header class="head">
      <div class="heading">
        <h1 dir="auto">{ticket.subject}</h1>
        <div class="meta">
          <span class="ref">{ticket.ref}</span>
          <span>{topicLabel(ticket.topic)}</span>
          {#if status}<StatusBadge kind={status.kind} label={status.label} />{/if}
        </div>
      </div>
      {#if !resolved}
        <Button onclick={askResolve}>{msg('tickets.resolve')}</Button>
      {/if}
    </header>

    <ul class="thread">
      {#each thread.messages as message (message.id)}
        <li class="bubble" class:mine={message.author === 'user'} class:staff={message.author !== 'user'}>
          <div class="bubble-head">
            <span class="author">{message.author === 'user' ? msg('tickets.you') : msg('tickets.staff')}</span>
            <span class="time">{when(message.createdAt)}</span>
          </div>
          <p class="text" dir="auto">{message.body}</p>
          {#if message.logBundleId}
            <span class="logs"><Paperclip size="1.4rem" strokeWidth={1.8} />{msg('tickets.logsAttached')}</span>
          {/if}
        </li>
      {/each}
    </ul>

    <div class="reply">
      {#if resolved}
        <p class="hint">{msg('tickets.reopenNote')}</p>
      {/if}
      {#if limitsFailure}
        <div class="banner" role="alert">
          <CircleAlert size="1.6rem" strokeWidth={1.8} />
          <span class="banner-text">{limitsFailure}</span>
          <Button size="sm" onclick={loadLimits}>{msg('common.retry')}</Button>
        </div>
      {:else if limits}
        <label class="field">
          <span class="label">
            {msg('tickets.replyLabel')}
            <span class="counter" class:over={bodyLength(reply) > limits.maxBodyRunes}>
              {msg('tickets.counter', { count: bodyLength(reply), max: limits.maxBodyRunes })}
            </span>
          </span>
          <textarea
            class="input area"
            class:invalid={!!replyError}
            rows="5"
            dir="auto"
            placeholder={msg('tickets.replyPlaceholder')}
            bind:value={reply}
            disabled={sending}
            oninput={() => (replyError = '')}
          ></textarea>
          {#if replyError}<span class="error" role="alert">{replyError}</span>{/if}
        </label>

        <AttachLogs bind:checked={attachLogs} disabled={sending} />

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
          <Button variant="primary" disabled={sending} onclick={submitReply}>
            {sending ? msg('tickets.replySending') : msg('tickets.reply')}
          </Button>
        </div>
      {/if}
    </div>
  {/if}
</Card>

<LogsConfirm bind:this={confirm} />

{#if resolving}
  <ConfirmModal prompt={resolving} onconfirm={confirmResolve} onclose={() => (resolving = null)} />
{/if}

<style>
  .top {
    margin: 0 0 var(--space-4) -0.8rem;
  }

  .muted {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .banner {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
    padding: 0.9rem 1.2rem;
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
    color: var(--danger);
    font-size: var(--font-sm);
  }

  .banner :global(svg) {
    flex-shrink: 0;
  }

  .banner-text {
    flex: 1;
    min-width: 0;
    line-height: 1.4;
  }

  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-6);
    margin-bottom: var(--space-6);
  }

  .heading {
    min-width: 0;
  }

  h1 {
    font-size: var(--font-title);
    font-weight: 600;
    line-height: 1.2;
    letter-spacing: var(--tracking-title);
    overflow-wrap: anywhere;
  }

  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem var(--space-3);
    margin-top: var(--space-2);
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .ref {
    font-variant-numeric: tabular-nums;
    color: var(--text-2);
  }

  .thread {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin: 0 0 var(--space-6);
    padding: 0;
    list-style: none;
  }

  .bubble {
    max-width: 68rem;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
  }

  .bubble.mine {
    align-self: flex-end;
    background: var(--accent-subtle);
    border-color: var(--accent);
  }

  .bubble.staff {
    align-self: flex-start;
  }

  .bubble-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: 0.6rem;
    font-size: var(--font-xs);
  }

  .author {
    font-weight: 600;
    color: var(--text);
  }

  .time {
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .text {
    margin: 0;
    font-size: var(--font-sm);
    line-height: 1.55;
    color: var(--text);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .logs {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.8rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .reply {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 72rem;
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.45;
    color: var(--text-3);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
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
    padding: 0.9rem 1.2rem;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    color: var(--text);
    font-size: var(--font-sm);
    font-family: inherit;
    line-height: 1.55;
    resize: vertical;
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

  .error {
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
  }
</style>
