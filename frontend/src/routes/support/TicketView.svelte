<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { ArrowLeft, Paperclip } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import TextArea from '../../lib/components/TextArea.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import type { ConfirmPrompt } from '../../lib/confirm/prompts';
  import { msg } from '../../lib/i18n';
  import { accountErrorField } from '../../lib/services/accountMessages';
  import { onLogUploadStatus, type LogUploadStatus } from '../../lib/services/logsUpload';
  import {
    getTicket,
    resolveTicket,
    ticketLimits,
    type Limits,
    type TicketThread,
  } from '../../lib/services/support';
  import { supportErrorText } from '../../lib/services/supportMessages';
  import { sendReply, validateBody } from '../../lib/support/draft';
  import { unreadGrew } from '../../lib/support/unread';
  import { logsStageText, uploadPercent } from '../../lib/support/progress';
  import { statusView, topicLabel } from '../../lib/support/topics';
  import { navigate } from '../../lib/stores/router';
  import { refreshUnread, unreadReplies } from '../../lib/stores/support';
  import { toast } from '../../lib/stores/toasts';
  import { handleSessionLost } from '../../lib/stores/user';
  import AttachLogs from './AttachLogs.svelte';
  import LogsConfirm from './LogsConfirm.svelte';
  import { messageWhen } from './messageTime';
  import { bodyCounter, isSendShortcut, sendKeys, sessionGone } from './sending';

  let { number }: { number: number } = $props();
  const uid = $props.id();

  let thread = $state<TicketThread | null>(null);
  let loading = $state(true);
  let failure = $state('');
  let limits = $state<Limits | null>(null);
  let limitsFailure = $state('');
  let reply = $state('');
  let replyError = $state('');
  let replyFailure = $state('');
  // A read or a reply found the session gone: its notice offers to sign in instead of a Retry.
  let signedOut = $state(false);
  let replySignedOut = $state(false);
  let attachLogs = $state(false);
  let sending = $state(false);
  let upload = $state<LogUploadStatus | null>(null);
  let resolving = $state<ConfirmPrompt | null>(null);
  let retrying = $state(false);
  let limitsRetrying = $state(false);
  let confirm: LogsConfirm;
  let run = 0;

  const ticket = $derived(thread?.ticket ?? null);
  const status = $derived(ticket ? statusView(ticket.status) : null);
  const resolved = $derived(ticket?.status === 'resolved');
  const stage = $derived(logsStageText(upload));

  let head = $state<HTMLElement>();
  let replyBox = $state<HTMLElement>();
  let replyField = $state<HTMLTextAreaElement>();

  // A failure stays on screen until an answer replaces it: a refresh that starts does not clear it.
  async function load(): Promise<boolean> {
    const current = ++run;
    try {
      const next = await getTicket(number);
      if (current !== run) return false;
      thread = next;
      failure = '';
      signedOut = false;
      void refreshUnread();
      return true;
    } catch (err) {
      if (current !== run) return false;
      failure = supportErrorText(err, msg('tickets.ticketLoadFailed'));
      signedOut = sessionGone(err);
      return false;
    } finally {
      if (current === run) loading = false;
    }
  }

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

  // A reload a reply asked for while a Retry or a sent reply was on its way: it runs once that answers.
  let refreshPending = false;

  function settle() {
    if (!refreshPending || sending || retrying) return;
    refreshPending = false;
    void load();
  }

  // The retry's own request may have left before the reply came: a reload held back meanwhile follows.
  async function retryThread(): Promise<boolean> {
    try {
      return await load();
    } finally {
      if (refreshPending) {
        refreshPending = false;
        void load();
      }
    }
  }

  let lastUnread: number | null = null;
  $effect(() => {
    const count = $unreadReplies;
    untrack(() => {
      const grew = unreadGrew(lastUnread, count);
      lastUnread = count;
      if (!grew || !thread) return;
      // A retry or a sent reply on its way is not overtaken: its notice stays and its answer is kept.
      if (sending || retrying) refreshPending = true;
      else void load();
    });
  });

  function sendOnShortcut(event: KeyboardEvent) {
    if (!isSendShortcut(event)) return;
    event.preventDefault();
    void submitReply();
  }

  // A refusal of the text is said at the box, which takes focus; any other once, in the reply form.
  async function submitReply() {
    if (sending || !limits) return;
    replyFailure = '';
    replyError = validateBody(reply, limits);
    if (replyError) {
      await tick();
      replyField?.focus();
      return;
    }
    // Focus in the reply form at the press (the box or Reply) goes to the cleared box once it is sent.
    const fromForm = Boolean(replyBox?.contains(document.activeElement));
    sending = true;
    upload = null;
    try {
      const outcome = await sendReply(number, reply, attachLogs, () => confirm.ask());
      if (!outcome.sent) return;
      reply = '';
      attachLogs = false;
      sending = false;
      toast(msg('tickets.replySent'), 'success');
      if (fromForm) {
        await tick();
        replyField?.focus();
      }
      refreshPending = false;
      await load();
    } catch (err) {
      sending = false;
      const text = supportErrorText(err, msg('tickets.replyFailed'));
      if (accountErrorField(err) === 'body') {
        replyError = text;
        await tick();
        replyField?.focus();
      } else {
        replyFailure = text;
        replySignedOut = sessionGone(err);
      }
    } finally {
      sending = false;
      upload = null;
      settle();
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

<div class="top">
  <Button variant="ghost" size="sm" onclick={() => navigate('support')}>
    <ArrowLeft size="1.5rem" strokeWidth={1.8} />
    {msg('tickets.back')}
  </Button>
</div>

{#if failure || retrying}
  <div class="banner">
    <Notice tone="danger" bind:retrying retry={signedOut ? undefined : { label: msg('common.retry'), run: retryThread, focus: () => head?.querySelector('h1') }} action={signedOut ? signIn : undefined}>
      {failure}
    </Notice>
  </div>
{/if}

{#snippet signIn()}
  <Button size="sm" onclick={handleSessionLost}>{msg('tickets.signInAgain')}</Button>
{/snippet}

{#if loading}
  <Spinner label={msg('tickets.loading')} />
{:else if ticket && thread}
  <header class="head" bind:this={head}>
    <div class="heading">
      <h1 dir="auto" tabindex="-1">{ticket.subject}</h1>
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

  <ul class="thread" aria-label={msg('tickets.thread')}>
    {#each thread.messages as message (message.id)}
      {@const written = messageWhen(message.createdAt)}
      <li class="bubble" class:mine={message.author === 'user'} class:staff={message.author !== 'user'}>
        <div class="bubble-head">
          <span class="author">{message.author === 'user' ? msg('tickets.you') : msg('tickets.staff')}</span>
          {#if written}
            <!-- The exact time goes to assistive tech as text and to the pointer as a hint; the time
                 element takes focus only from the pointer, so a message adds no Tab stop for it. -->
            <Tooltip text={written.exact} describe={false}>
              <time class="time" datetime={message.createdAt} tabindex="-1"><span aria-hidden="true">{written.words}</span><span class="sr-only">{written.exact}</span></time>
            </Tooltip>
          {/if}
        </div>
        <p class="text" dir="auto">{message.body}</p>
        {#if message.logBundleId}
          <span class="logs"><Paperclip size="1.4rem" strokeWidth={1.8} />{msg('tickets.logsAttached')}</span>
        {/if}
      </li>
    {/each}
  </ul>

  <div class="reply" bind:this={replyBox}>
    {#if resolved}
      <p class="hint">{msg('tickets.reopenNote')}</p>
    {/if}
    {#if limitsFailure || limitsRetrying}
      <Notice tone="danger" bind:retrying={limitsRetrying} retry={{ label: msg('common.retry'), run: loadLimits, within: () => replyBox }}>
        {limitsFailure}
      </Notice>
    {:else if limits}
      <!-- Read-only, not disabled, while the reply is on its way: the box keeps the focus a send
           shortcut was pressed in. -->
      <TextArea
        id="{uid}-reply"
        label={msg('tickets.replyLabel')}
        rows={5}
        grow={12}
        placeholder={msg('tickets.replyPlaceholder')}
        counter={bodyCounter(limits.maxBodyRunes)}
        hint={msg('tickets.sendHint', { keys: sendKeys() })}
        readonly={sending}
        error={replyError}
        bind:value={reply}
        bind:textarea={replyField}
        oninput={() => (replyError = '')}
        onkeydown={sendOnShortcut}
      />

      <AttachLogs bind:checked={attachLogs} disabled={sending} />

      {#if replyFailure}
        <Notice tone="danger" compact action={replySignedOut ? signIn : undefined}>{replyFailure}</Notice>
      {/if}

      {#if sending && stage}
        <div class="progress">
          <span class="hint">{stage}</span>
          <ProgressBar
            value={upload ? uploadPercent(upload) : 0}
            indeterminate={upload?.state !== 'sending' || !upload.totalBytes}
            height={5}
            label={msg('settings.aboutLogsLabel')}
            valueText={stage}
          />
        </div>
      {/if}

      <div class="actions">
        <Button variant="primary" busy={sending} onclick={submitReply}>
          {sending ? msg('tickets.replySending') : msg('tickets.reply')}
        </Button>
      </div>
    {/if}
  </div>
{/if}

<LogsConfirm bind:this={confirm} />

{#if resolving}
  <ConfirmModal prompt={resolving} onconfirm={confirmResolve} onclose={() => (resolving = null)} />
{/if}

<style>
  .top {
    margin: 0 0 var(--space-4) calc(-1 * var(--space-2));
  }

  .banner {
    margin-bottom: var(--space-4);
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
    gap: var(--space-2) var(--space-3);
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
    border-color: var(--accent-bright);
  }

  .bubble.staff {
    align-self: flex-start;
  }

  .bubble-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-2);
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
    gap: var(--space-1);
    margin-top: var(--space-2);
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

  .progress {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
  }
</style>
