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
  import { focusQuietly } from '../../lib/layers/focus';
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
  import { attachLogsByDefault, isTopic, parseTopic, topicOptions } from '../../lib/support/topics';
  import { goBack, navigate } from '../../lib/stores/router';
  import { handleSessionLost } from '../../lib/stores/user';
  import { runeCount } from '../../lib/profile/layout';
  import AttachLogs from './AttachLogs.svelte';
  import {
    account,
    accountToast,
    forgetRequest,
    keepRequest,
    offeredRequests,
    onRequestBack,
    openPlain,
    openPrefilled,
    requestsChanged,
    restoreRequest,
    returnRequest,
    returnShown,
    type Offer,
    type Placed,
    type RequestDraft,
    type Writer,
  } from './drafts';
  import LogsConfirm from './LogsConfirm.svelte';
  import { bodyCounter, copyAction, isPlainEnter, isSendShortcut, sendKeys, sessionGone } from './sending';

  // A way in that sets a topic or a subject (the Beta badge, the Move dialog, Settings) passes it; the
  // list's "New request" passes neither.
  let { initialTopic, initialSubject }: { initialTopic?: SupportTopic; initialSubject?: string } = $props();
  const uid = $props.id();

  // This form, as the writer of what is kept: its Cancel and its own send forget only what it wrote.
  const me: Writer = {};

  // What the player left unsent (the session gone, another screen opened): a plain way in gives it back
  // whole; a way in with a prefill opens with the prefill and offers it. Every other unsent request is
  // offered beside it, each on its own line, from what is kept, so the offers outlive the form.
  const prefill = untrack(() => ({ topic: initialTopic, subject: initialSubject }));
  const prefilled = prefill.topic !== undefined || prefill.subject !== undefined;
  const startTopic = prefill.topic ?? parseTopic(undefined);
  const draft = prefilled ? null : openPlain(me);
  if (prefilled) openPrefilled(me);
  const offers = $derived.by(() => {
    void $requestsChanged;
    return offeredRequests(me);
  });
  // The offers the form opened with, or the ones Restore left, are standing facts of the page; one that
  // came later (a failed send handed it in) is said once, politely.
  let quietOffers = $state(offeredRequests(me).map((offer) => offer.id));
  let topic = $state<string>(draft?.topic ?? startTopic);
  let attachLogs = $state(draft?.attachLogs ?? attachLogsByDefault(startTopic));
  let logsTouched = draft?.logsTouched ?? false;
  let game = $state<{ igdbId: number; title: string } | null>(draft?.game ?? null);
  let pickerOpen = $state(false);
  let subject = $state(draft?.subject ?? prefill.subject ?? '');
  let body = $state(draft?.body ?? '');
  let limits = $state<Limits | null>(null);
  let limitsFailure = $state('');
  let errors = $state<DraftErrors>({});
  let formError = $state('');
  // The send found the session gone: the notice offers to sign in.
  let formSignedOut = $state(false);
  let sending = $state(false);
  let limitsRetrying = $state(false);
  let upload = $state<LogUploadStatus | null>(null);
  let confirm: LogsConfirm;

  const topics = $derived(topicOptions());
  const stage = $derived(logsStageText(upload));

  // This form's content is what is kept: it gave back the draft, or the player typed or chose content
  // (a subject, a text, a game) or restored one. The topic and the logs switch are settings: they are
  // carried into the draft once there is one, and never make one. A prefill alone is never kept.
  let owns = $state(draft !== null);
  // A topic and the logs switch are settings, not content.
  const holdsNothing = $derived(!subject.trim() && !body.trim() && !game);
  const holdsTyped = $derived(owns && !holdsNothing);

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

  function current(): RequestDraft {
    return { topic, game: game ? { ...game } : null, subject, body, attachLogs, logsTouched };
  }

  function fill(from: RequestDraft) {
    topic = from.topic;
    game = from.game;
    subject = from.subject;
    body = from.body;
    attachLogs = from.attachLogs;
    logsTouched = from.logsTouched;
  }

  onMount(() => {
    void loadLimits();
    const off = onLogUploadStatus((status) => {
      if (sending) upload = status;
    });
    // A request of another form whose send failed comes into this one when it holds nothing, as its
    // content; otherwise what is kept places it, and the notice offers it.
    const offBack = onRequestBack({
      by: me,
      free: () => holdsNothing && !sending,
      sending: () => sending,
      shown: () => limits !== null && !limitsFailure && !limitsRetrying,
      take: (back) => {
        fill(back);
        owns = true;
      },
    });
    return () => {
      onScreen = false;
      off();
      offBack();
    };
  });

  // Nothing is written while this form's request is on its way: its text left the draft when the send
  // started, and only a failed send gives it back.
  $effect(() => {
    const now = { topic, game, subject, body, attachLogs };
    if (!owns || untrack(() => sending)) return;
    untrack(() => {
      const dropped = keepRequest({ ...now, logsTouched }, me);
      if (dropped) sayDropped(dropped);
    });
  });

  // From the first content the player gives it this form's content is the draft. A draft another form
  // wrote moves to the spare and stays offered; with the spare taken it has no place left, so the first
  // edit drops that line of the notice and the toast names the draft and offers to copy it.
  function edit() {
    owns = true;
  }

  // A kept text that found no place left is not lost unsaid: the toast names it and copies it.
  function sayDropped(text: RequestDraft) {
    const subject = text.subject.trim();
    const words = subject ? msg('tickets.draftDropped', { subject }) : msg('tickets.draftDroppedNoSubject');
    const who = account();
    accountToast(who, words, 'danger', copyAction(`${subject}\n\n${text.body.trim()}`.trim(), who));
  }

  // A failed send's toast says only where its text is: "back in the form" while a form on screen shows
  // it, with nothing more to do; "you can restore it" while the notice on screen names it; else kept, or
  // kept nowhere. Each of those offers to copy it: the text is not on screen.
  function sayPlaced(placed: Placed, sent: RequestDraft, who: string | null) {
    const subject = sent.subject.trim();
    const copy = copyAction(`${subject}\n\n${sent.body.trim()}`, who);
    if (placed === 'form') accountToast(who, msg('tickets.requestNotSentKept', { subject }), 'danger');
    else if (placed === 'offered') accountToast(who, msg('tickets.requestNotSentOffered', { subject }), 'danger', copy);
    else if (placed === 'kept') accountToast(who, msg('tickets.requestNotSentSaved', { subject }), 'danger', copy);
    else accountToast(who, msg('tickets.requestNotSentNotKept', { subject }), 'danger', copy);
  }

  // What the player typed is not lost to a restore: it is offered in turn, from what is kept.
  async function restore(id: number) {
    if (sending) return;
    const back = restoreRequest(me, holdsTyped ? current() : null, id);
    if (!back) return;
    fill(back.draft);
    owns = true;
    quietOffers = offeredRequests(me).map((offer) => offer.id);
    if (back.dropped) sayDropped(back.dropped);
    // The notice's button may leave: focus goes on to the subject, never to the page.
    await tick();
    subjectField?.focus();
  }

  // The swap sentence is true where Restore keeps the typed text: it takes the spare's place, or the free
  // spare when the draft another form wrote is restored.
  function swaps(offer: Offer): boolean {
    return holdsTyped && (offer.spare || !offers.some((other) => other.spare));
  }

  function cancel() {
    forgetRequest(me);
    goBack();
  }

  function changeTopic(id: string) {
    topic = id;
    if (!logsTouched && isTopic(id)) attachLogs = attachLogsByDefault(id);
  }

  function clearGame() {
    edit();
    game = null;
  }

  function editField(field: 'subject' | 'body') {
    edit();
    errors = { ...errors, [field]: undefined };
  }

  function setLogs() {
    logsTouched = true;
  }

  function pickGame(pick: { igdbId: number; game: { title: string } }) {
    edit();
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
    else {
      formError = text;
      formSignedOut = sessionGone(err);
    }
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
    // Every control but Send is held while the request is on its way. One that has focus (a click on Send
    // in the macOS WebKit view takes none) would drop it to the page as it turns disabled: Send keeps it.
    const focused = document.activeElement;
    const send = document.getElementById(`${uid}-send`);
    if (focused instanceof HTMLButtonElement && focused !== send && form?.contains(focused) && send) focusQuietly(send);
    sending = true;
    upload = null;
    const sent = current();
    let started = false;
    let who: string | null = null;
    try {
      const outcome = await sendNewTicket(
        { topic, subject, body, gameIgdbId: game?.igdbId ?? null, attachLogs },
        () => confirm.ask(),
        () => {
          started = true;
          who = account();
          forgetRequest(me);
        },
      );
      if (!outcome.sent) return;
      // An answer that comes after another account signed in shows that account nothing of this request.
      if (account() !== who) return;
      forgetRequest(me);
      announceCreated(outcome.value, onScreen, {
        notify: (text) => accountToast(who, text, 'success'),
        open: (number) => navigate('support', { ticket: String(number) }),
      });
    } catch (err) {
      const changed = started && account() !== who;
      if (!onScreen || changed) {
        // Under another account nothing is kept; with no one signed in it is kept for `who`, unsaid.
        const placed = started ? returnRequest(sent, me, who) : null;
        if (placed) sayPlaced(placed, sent, who);
        return;
      }
      // The failed text is what the form shows: it is the draft, whatever was kept as one meanwhile.
      const dropped = started ? returnShown(sent, me, who) : null;
      if (dropped) sayDropped(dropped);
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
    {#each offers as offer (offer.id)}
      {#snippet restoreOffer()}
        <Button size="sm" aria-describedby="{uid}-offer-{offer.id}" disabled={sending} onclick={() => restore(offer.id)}>{msg('tickets.draftRestore')}</Button>
      {/snippet}
      <Notice tone="info" announce={!quietOffers.includes(offer.id)} action={restoreOffer}>
        <span id="{uid}-offer-{offer.id}">
          {offer.draft.subject.trim() ? msg('tickets.draftKept', { subject: offer.draft.subject.trim() }) : msg('tickets.draftKeptNoSubject')}
          {#if swaps(offer)}{msg('tickets.draftSwapNote')}{/if}
        </span>
      </Notice>
    {/each}

    <div class="field">
      <span class="label" id="{uid}-topic-label">{msg('tickets.topicLabel')}</span>
      <Select options={topics} value={topic} width="100%" labelledby="{uid}-topic-label" disabled={sending} onchange={changeTopic} />
    </div>

    <div class="field">
      <span class="label">{msg('tickets.gameLabel')}</span>
      {#if game}
        <div class="chosen">
          <span class="chosen-title" dir="auto">{game.title}</span>
          <IconButton size="sm" label={msg('tickets.gameClear')} disabled={sending} onclick={clearGame}>
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
      oninput={() => editField('subject')}
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
      oninput={() => editField('body')}
      onkeydown={sendOn(isSendShortcut)}
    />

    <AttachLogs
      bind:checked={attachLogs}
      disabled={sending}
      onchange={setLogs}
    />

    {#if formError}
      <Notice tone="danger" action={formSignedOut ? signIn : undefined}>{formError}</Notice>
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
      <Button disabled={sending} onclick={cancel}>{msg('common.cancel')}</Button>
      <Button id="{uid}-send" variant="primary" busy={sending} onclick={submit}>
        {sending ? msg('tickets.submitting') : msg('tickets.submit')}
      </Button>
    </div>
  </div>
{/if}

<LogsConfirm bind:this={confirm} />

{#snippet signIn()}
  <Button size="sm" onclick={handleSessionLost}>{msg('tickets.signInAgain')}</Button>
{/snippet}

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
    gap: var(--space-2);
    min-width: 0;
  }

  .label {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-2);
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
    padding: var(--space-1) var(--space-1) var(--space-1) var(--space-3);
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
    gap: var(--space-2);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

</style>
