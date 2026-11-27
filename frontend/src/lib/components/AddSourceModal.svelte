<script lang="ts">
  import { sourceWarningText } from '../sources/sourceWarnings';
  import { untrack } from 'svelte';
  import { add as addSource, addFile as addSourceFile, errorMark, sourceErrorText, toastUnannounced } from '../stores/sources';
  import { selectFeedFile, testSource, testSourceFile, type SourcePreview } from '../services/sources';
  import { toast } from '../stores/toasts';
  import { focusHandOff } from '../utils/focusHandOff';
  import { composing } from '../utils/shortcut';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import PathField from './PathField.svelte';
  import Spinner from './Spinner.svelte';
  import TextField from './TextField.svelte';
  import { msg } from '../i18n';

  /**
   * A feed by URL or from a file: nothing is read until Check, which shows what the feed holds, and
   * nothing is added until Add. A failure is said at the field it is about, in the launcher's words for
   * its code, and the typed URL stays. The focus follows the step: the field of a failure, Add once the
   * preview is in.
   */
  let { open = $bindable(false) }: { open?: boolean } = $props();

  const uid = $props.id();
  const addId = `${uid}-add`;

  let step = $state<'input' | 'preview'>('input');
  let mode = $state<'url' | 'file'>('url');
  let url = $state('');
  let path = $state('');
  let preview = $state<SourcePreview | null>(null);
  let error = $state('');
  // Which field the failure is about: the URL typed or the file picked.
  let errorAt = $state<'url' | 'file'>('url');
  let checking = $state(false);
  let adding = $state(false);
  let run = 0;
  let urlInput = $state<HTMLInputElement>();
  let fileBlock = $state<HTMLElement>();

  function reset() {
    run++;
    step = 'input';
    mode = 'url';
    url = '';
    path = '';
    preview = null;
    error = '';
    errorAt = 'url';
    checking = false;
    adding = false;
  }

  $effect(() => {
    const isOpen = open;
    untrack(() => {
      if (isOpen) reset();
    });
  });

  // The control that says a failure about the URL or the file: the URL field, or the file's Browse.
  const fieldOf = (at: 'url' | 'file') => (at === 'url' ? urlInput : fileBlock?.querySelector<HTMLElement>('button'));

  // The focus the press holds goes on with the step; a press that held none moves nothing.
  function pressHand() {
    const pressed = document.activeElement;
    return focusHandOff(() => pressed);
  }

  function fail(at: 'url' | 'file', text: string) {
    errorAt = at;
    error = text;
  }

  async function pickFile() {
    let selected = '';
    try {
      selected = await selectFeedFile();
    } catch (err) {
      fail('file', sourceErrorText(err, msg('modals.addSourcePickFailed')));
      return;
    }
    if (!selected) return;
    // One of the two is checked and added: the file chosen takes the URL's place, and a URL typed
    // after takes the file's.
    path = selected;
    url = '';
    mode = 'file';
    await check();
  }

  async function check() {
    const value = mode === 'file' ? path : url.trim();
    if (!value) return;
    const token = run;
    const at = mode;
    const hand = pressHand();
    checking = true;
    error = '';
    try {
      const result = mode === 'file' ? await testSourceFile(value) : await testSource(value);
      if (token !== run) return;
      preview = result;
      step = 'preview';
      checking = false;
      await hand(() => document.getElementById(addId));
    } catch (err) {
      if (token !== run) return;
      checking = false;
      fail(at, sourceErrorText(err, msg('modals.addSourceCheckFailed')));
      await hand(() => fieldOf(at));
    }
  }

  async function submit() {
    const value = mode === 'file' ? path : url.trim();
    if (!value) return;
    const token = run;
    const at = mode;
    const hand = pressHand();
    const mark = errorMark();
    adding = true;
    try {
      const { source, failure } = mode === 'file' ? await addSourceFile(value) : await addSource(value);
      // Saved with its first refresh failed: the list shows it with that error and its own Refresh.
      if (failure !== undefined) toast(msg('modals.addSourceAddedRefreshFailed', { name: source.name, reason: sourceErrorText(failure) }), 'warning');
      else toast(msg('modals.addSourceAdded', { name: source.name }), 'success');
      if (token === run) open = false;
    } catch (err) {
      if (token === run && open) {
        error = sourceErrorText(err, msg('modals.addSourceAddFailed'));
        errorAt = at;
        step = 'input';
        adding = false;
        await hand(() => fieldOf(at));
      } else {
        toastUnannounced(err, mark);
      }
    } finally {
      if (token === run) adding = false;
    }
  }

  function typed() {
    mode = 'url';
    path = '';
  }

  // Enter in the URL field is Check's press while Check is live; an input method's Enter is its own.
  function onUrlKey(event: KeyboardEvent) {
    if (event.key !== 'Enter' || composing(event) || checking || !url.trim()) return;
    event.preventDefault();
    void check();
  }

  function back() {
    const hand = pressHand();
    preview = null;
    step = 'input';
    void hand(() => fieldOf(mode));
  }

  const location = $derived(preview ? (preview.type === 'file' ? preview.path : preview.url) : '');
  const heldReason = $derived(checking ? msg('modals.actionRunning') : undefined);
</script>

<!-- dismissible while adding: the add goes on in the background (the footer offers Minimize) and the sources store reports how it ended. -->
<Modal bind:open busy={checking} title={msg('modals.addSourceTitle')}>
  {#if step === 'input'}
    <div class="form">
      <TextField
        label={msg('modals.addSourceUrlLabel')}
        type="url"
        placeholder="https://example.com/feed.json"
        bind:value={url}
        bind:input={urlInput}
        readonly={checking}
        error={errorAt === 'url' ? error : ''}
        oninput={typed}
        onkeydown={onUrlKey}
      />
      <div class="or" aria-hidden="true">
        <span class="line"></span>
        <span class="or-text">{msg('modals.addSourceOr')}</span>
        <span class="line"></span>
      </div>
      <div bind:this={fileBlock}>
        <PathField
          label={msg('modals.addSourceFileLabel')}
          value={path}
          placeholder={msg('modals.addSourceNoFile')}
          browseLabel={msg('modals.addSourcePickFile')}
          onbrowse={pickFile}
          disabledReason={heldReason}
          error={errorAt === 'file' ? error : ''}
        />
      </div>
      {#if checking}
        <div class="checking" role="status">
          <Spinner size="sm" />
          <span class="checking-text">{msg('modals.addSourceChecking')}</span>
          <span class="checking-note">{msg('modals.addSourceCheckingNote')}</span>
        </div>
      {/if}
    </div>
  {:else if preview}
    <div class="preview">
      <div class="preview-row">
        <span class="key">{msg('modals.addSourceName')}</span>
        <span class="value">{preview.name}</span>
      </div>
      <div class="preview-row">
        <span class="key">{preview.type === 'file' ? msg('modals.addSourceFileLabel') : 'URL'}</span>
        <span class="value location">{location}</span>
      </div>
      <div class="preview-row">
        <span class="key">{msg('modals.addSourceFeedVersion')}</span>
        <span class="value">{preview.feedVersion}</span>
      </div>
      <div class="preview-row">
        <span class="key">{msg('modals.addSourceEntries')}</span>
        <span class="value">{preview.entries}</span>
      </div>
      <div class="preview-row">
        <span class="key">{msg('modals.addSourceInvalidEntries')}</span>
        <span class="value">{preview.invalid}</span>
      </div>
      <div class="preview-row">
        <span class="key">{msg('modals.addSourceGames')}</span>
        <span class="value">{preview.games}</span>
      </div>
      <div class="preview-row">
        <span class="key">{msg('modals.addSourceKnown')}</span>
        <span class="value">{preview.known}</span>
      </div>
      <div class="preview-row">
        <span class="key">{msg('modals.addSourceUnknown')}</span>
        <span class="value">{preview.unknown}</span>
      </div>
      {#if preview.insecure || preview.duplicate || preview.warnings?.length}
        <div class="warnings">
          {#if preview.insecure}
            <Notice tone="warning" compact announce={false}>{msg('modals.addSourceInsecure')}</Notice>
          {/if}
          {#if preview.duplicate}
            <Notice tone="warning" compact announce={false}>{msg('modals.addSourceDuplicate')}</Notice>
          {/if}
          {#each preview.warnings ?? [] as warning (warning)}
            <Notice tone="warning" compact announce={false}>{sourceWarningText(warning)}</Notice>
          {/each}
        </div>
      {/if}
      {#if adding}
        <p class="background-note">{msg('modals.addSourceBackgroundNote')}</p>
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    {#if step === 'input'}
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={!url.trim() && !path} reason={msg('modals.addSourceNeedInput')} busy={checking} onclick={check}>
        {msg('modals.addSourceCheck')}
      </Button>
    {:else if step === 'preview'}
      {#if adding}
        <Button onclick={() => (open = false)}>{msg('modals.addSourceMinimize')}</Button>
      {:else}
        <Button onclick={back}>{msg('common.back')}</Button>
      {/if}
      <Button variant="primary" id={addId} busy={adding} onclick={submit}>{adding ? msg('modals.addSourceAdding') : msg('common.add')}</Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .or {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .line {
    flex: 1;
    height: 1px;
    background: var(--border);
  }

  .or-text {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .checking {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
  }

  .checking-text {
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .checking-note {
    flex-basis: 100%;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .preview {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.4rem var(--space-4);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .preview-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.9rem 0;
    font-size: var(--font-sm);
  }

  .preview-row + .preview-row {
    border-top: 1px solid var(--border);
  }

  .key {
    flex-shrink: 0;
    color: var(--text-3);
  }

  .value {
    min-width: 0;
    color: var(--text);
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  /* A long URL or path wraps: the whole of it is on screen, not behind a hover. */
  .location {
    overflow-wrap: anywhere;
  }

  .background-note {
    margin-top: 0.6rem;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .warnings {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    margin: 0.6rem 0;
  }
</style>
