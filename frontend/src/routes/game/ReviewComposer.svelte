<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import { AccountError } from '../../lib/services/account';
  import {
    limits as fetchLimits,
    mine as fetchMine,
    remove as removeReview,
    save as saveReview,
    type Eligibility,
    type Limits,
    type Mine,
  } from '../../lib/services/reviews';
  import { eligibilityMessage } from '../../lib/game/reviewEligibility';
  import { reviewFieldMessage, reviewToastMessage } from '../../lib/game/reviewMessages';
  import { notifyReviewsChanged } from '../../lib/game/reviewsRefresh';
  import { toast } from '../../lib/stores/toasts';
  import { authState } from '../../lib/stores/user';
  import { msg, type MessageKey } from '../../lib/i18n';
  import { focusableIn } from '../../lib/layers/focus';
  import { nextIndex } from '../../lib/utils/roving';
  import { focusAfterRetry } from './retryFocus';

  let { canonicalGameId }: { canonicalGameId: string } = $props();

  const uid = $props.id();

  const VERDICTS: { value: boolean; mark: string; label: MessageKey }[] = [
    { value: true, mark: '👍', label: 'reviews.composerRecommend' },
    { value: false, mark: '👎', label: 'reviews.composerNotRecommend' },
  ];

  let verdictGroup = $state<HTMLElement>();

  // A radio group: the arrows move the choice and the focus together, wrapping at the ends.
  function moveVerdict(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const radios = [...(verdictGroup?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];
    const next = nextIndex(radios.indexOf(event.currentTarget as HTMLElement), VERDICTS.length, event.key, { orientation: 'both' });
    if (next === null) return;
    event.preventDefault();
    recommended = VERDICTS[next].value;
    radios[next]?.focus();
  }

  const RETRY_CODES = new Set([
    'account_muted',
    'review_account_too_new',
    'review_not_played',
    'review_repost_cooldown',
    'review_post_cooldown',
    'review_daily_limit',
    'review_edit_cooldown',
  ]);

  let phase = $state<'loading' | 'error' | 'ready'>('loading');
  let mine = $state<Mine | null>(null);
  let limits = $state<Limits | null>(null);
  let editing = $state(false);
  let recommended = $state<boolean | null>(null);
  let body = $state('');
  let saving = $state(false);
  let confirmDeleteOpen = $state(false);
  let fieldError = $state('');

  const authed = $derived($authState === 'authenticated');
  const isEdit = $derived(!!mine?.review);
  const formVisible = $derived(!!mine && ((!mine.review && mine.eligibility.canPost) || editing));
  const runeCount = $derived([...body].length);
  const disabledPublish = $derived(
    recommended === null || saving || !limits || runeCount < limits.minBodyRunes || runeCount > limits.maxBodyRunes,
  );

  let loadToken = 0;

  // A retry keeps the failure on screen until its answer: the notice and its focused button stay.
  async function load(id: string, retry = false) {
    const token = ++loadToken;
    if (!retry) phase = 'loading';
    try {
      const [mineResult, limitsResult] = await Promise.all([fetchMine(id), fetchLimits()]);
      if (token !== loadToken) return;
      mine = mineResult;
      limits = limitsResult;
      editing = false;
      recommended = null;
      body = '';
      fieldError = '';
      phase = 'ready';
    } catch {
      if (token === loadToken) phase = 'error';
    }
  }

  let retrying = $state(false);
  let root = $state<HTMLElement>();

  async function retry() {
    if (retrying) return;
    const button = document.activeElement;
    retrying = true;
    try {
      await load(canonicalGameId, true);
    } finally {
      retrying = false;
    }
    await focusAfterRetry(button, () => (root ? (focusableIn(root)[0] ?? root) : undefined));
  }

  $effect(() => {
    const id = canonicalGameId;
    if (!authed || !id) {
      loadToken += 1;
      mine = null;
      phase = 'ready';
      return;
    }
    void load(id);
  });

  $effect(() => {
    const id = canonicalGameId;
    const retryAt = mine?.eligibility.canPost === false ? Date.parse(mine.eligibility.retryAt) : NaN;
    if (!authed || !id || Number.isNaN(retryAt)) return;
    const delay = Math.min(Math.max(retryAt - Date.now() + 1000, 1000), 2_147_000_000);
    const timer = setTimeout(() => void load(id), delay);
    return () => clearTimeout(timer);
  });

  function describeEligibility(elig: Eligibility): string {
    const result = eligibilityMessage(elig);
    if (result.kind === 'muted' || result.kind === 'unavailable') return msg(result.key);
    if (result.kind === 'playtime') return msg(result.key, result.params);
    if (result.kind === 'retry') return msg(result.key, { time: msg(result.retry.key, result.retry.params) });
    return '';
  }

  function startEdit() {
    if (!mine?.review) return;
    recommended = mine.review.recommended;
    body = mine.review.body;
    fieldError = '';
    editing = true;
  }

  function cancelEdit() {
    editing = false;
    fieldError = '';
  }

  async function submit() {
    if (disabledPublish || recommended === null) return;
    saving = true;
    fieldError = '';
    try {
      await saveReview(canonicalGameId, recommended, body);
      notifyReviewsChanged();
      void load(canonicalGameId);
    } catch (err) {
      const code = err instanceof AccountError ? err.code : '';
      const field = err instanceof AccountError ? err.field : '';
      if (field === 'body') {
        fieldError = limits ? reviewFieldMessage(code, limits) : msg('reviews.errorGeneric');
      } else if (RETRY_CODES.has(code)) {
        void load(canonicalGameId);
        toast(reviewToastMessage(code, msg('reviews.errorGeneric')), 'danger');
      } else {
        toast(reviewToastMessage(code, msg('reviews.errorGeneric')), 'danger');
      }
    } finally {
      saving = false;
    }
  }

  async function confirmDelete() {
    await removeReview(canonicalGameId);
    notifyReviewsChanged();
    void load(canonicalGameId);
  }

  function deleteErrorText(err: unknown): string {
    return reviewToastMessage(err instanceof AccountError ? err.code : '', msg('reviews.composerDeleteError'));
  }
</script>

<!-- Focusable by script only: focus lands here when a retry brings content without a control. -->
<div tabindex="-1" bind:this={root}>
  {#if !authed}
    <p class="prompt">{msg('reviews.composerSignInPrompt')}</p>
  {:else if phase === 'loading'}
    <Spinner size="sm" label={msg('reviews.loading')} />
  {:else if phase === 'error'}
    <Notice tone="danger">
      {msg('reviews.loadError')}
      {#snippet action()}
        <Button size="sm" busy={retrying} onclick={retry}>{msg('common.retry')}</Button>
      {/snippet}
    </Notice>
  {:else if mine}
    {#if formVisible}
      <div class="form">
        <div
          class="toggles"
          role="radiogroup"
          aria-label={msg('reviews.composerMineTitle')}
          aria-describedby={recommended === null && !fieldError ? `${uid}-choose` : undefined}
          bind:this={verdictGroup}
        >
          {#each VERDICTS as verdict, index (verdict.value)}
            <button
              type="button"
              class="toggle"
              class:positive={verdict.value}
              class:negative={!verdict.value}
              class:active={recommended === verdict.value}
              role="radio"
              aria-checked={recommended === verdict.value}
              tabindex={recommended === verdict.value || (recommended === null && index === 0) ? 0 : -1}
              onclick={() => (recommended = verdict.value)}
              onkeydown={moveVerdict}
            >
              <span aria-hidden="true">{verdict.mark}</span>
              {msg(verdict.label)}
            </button>
          {/each}
        </div>

        <textarea
          class="body"
          rows="5"
          placeholder={msg('reviews.composerPlaceholder')}
          bind:value={body}
          oninput={() => (fieldError = '')}
        ></textarea>

        <div class="form-foot">
          <span class="hint" class:danger={fieldError}>
            {fieldError || (limits ? msg('reviews.composerHint', { min: limits.minBodyRunes, max: limits.maxBodyRunes }) : '')}
          </span>
          {#if limits}
            <span class="counter" class:danger={runeCount > limits.maxBodyRunes}>
              {msg('reviews.composerCounter', { count: runeCount, max: limits.maxBodyRunes })}
            </span>
          {/if}
        </div>

        <div class="actions">
          {#if isEdit}
            <Button variant="ghost" disabled={saving} onclick={cancelEdit}>{msg('reviews.composerCancel')}</Button>
          {/if}
          <Button variant="primary" disabled={disabledPublish} onclick={submit}>
            {#if isEdit}
              {saving ? msg('reviews.composerSaving') : msg('reviews.composerSave')}
            {:else}
              {saving ? msg('reviews.composerPublishing') : msg('reviews.composerPublish')}
            {/if}
          </Button>
        </div>
        {#if recommended === null && !fieldError}
          <p class="choose-hint" id="{uid}-choose">{msg('reviews.composerChooseFirst')}</p>
        {/if}
      </div>
    {:else if mine.review}
      <div class="mine">
        <div class="mine-head">
          <span class="recommend" class:positive={mine.review.recommended} class:negative={!mine.review.recommended}>
            {mine.review.recommended ? msg('reviews.recommends') : msg('reviews.notRecommends')}
          </span>
          <h4 class="mine-title">{msg('reviews.composerMineTitle')}</h4>
        </div>
        <p class="mine-body">{mine.review.body}</p>
        {#if mine.review.hidden}
          <div class="hidden-note"><Notice tone="warning" compact announce={false}>{msg('reviews.composerHiddenNote')}</Notice></div>
        {/if}
        <div class="mine-actions">
          {#if mine.eligibility.canPost}
            <Button size="sm" onclick={startEdit}>{msg('reviews.composerEdit')}</Button>
          {/if}
          <Button size="sm" variant="danger" onclick={() => (confirmDeleteOpen = true)}>
            {msg('reviews.composerDelete')}
          </Button>
        </div>
        {#if !mine.eligibility.canPost}
          <p class="eligibility-note">{describeEligibility(mine.eligibility)}</p>
        {/if}
      </div>
    {:else}
      <p class="eligibility-note">{describeEligibility(mine.eligibility)}</p>
    {/if}
  {/if}
</div>

{#if confirmDeleteOpen}
  <ConfirmModal
    prompt={{
      title: msg('reviews.composerDeleteConfirmTitle'),
      text: msg('reviews.composerDeleteConfirmText'),
      confirm: msg('reviews.composerDeleteConfirmButton'),
    }}
    onconfirm={confirmDelete}
    onclose={() => (confirmDeleteOpen = false)}
    errorText={deleteErrorText}
  />
{/if}

<style>
  .prompt {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .toggles {
    display: flex;
    gap: var(--space-2);
  }

  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: var(--control-md);
    padding: 0 1.4rem;
    border-radius: var(--radius-md);
    border: 1px solid var(--border-strong);
    font-size: var(--font-sm);
    color: var(--text-2);
    transition: background var(--dur) var(--ease), border-color var(--dur) var(--ease), color var(--dur) var(--ease);
  }

  .toggle:hover {
    background: var(--hover-strong);
  }

  .toggle.positive.active {
    background: color-mix(in srgb, var(--accent-bright) 8%, var(--surface-2));
    border-color: var(--accent-bright);
    color: var(--accent-text);
  }

  .toggle.negative.active {
    background: color-mix(in srgb, var(--danger) 8%, var(--surface-2));
    border-color: var(--danger);
    color: var(--danger);
  }

  .body {
    width: 100%;
    padding: 1.2rem 1.4rem;
    border-radius: var(--radius-md);
    border: 1px solid var(--border-strong);
    background: var(--surface-3);
    color: var(--text);
    font: inherit;
    font-size: var(--font-sm);
    line-height: 1.55;
    resize: vertical;
  }

  .body:focus {
    outline: none;
    border-color: var(--accent-ring);
  }

  .form-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .hint.danger {
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
    background: var(--surface-2);
    color: var(--danger);
  }

  .counter.danger {
    background: var(--surface-2);
    color: var(--danger);
  }

  .counter {
    flex-shrink: 0;
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .choose-hint {
    font-size: var(--font-xs);
    color: var(--text-3);
    text-align: right;
  }

  .mine {
    padding: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--surface-3);
  }

  .mine-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .mine-title {
    font-size: var(--font-sm);
    font-weight: 600;
  }

  .recommend {
    font-size: var(--font-xs);
    font-weight: 500;
  }

  .recommend.positive {
    color: var(--accent-text);
  }

  .recommend.negative {
    color: var(--danger);
  }

  .mine-body {
    margin-top: var(--space-2);
    font-size: var(--font-sm);
    line-height: 1.6;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .hidden-note {
    margin-top: var(--space-2);
  }

  .mine-actions {
    display: flex;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .eligibility-note {
    margin-top: var(--space-2);
    font-size: var(--font-sm);
    color: var(--text-3);
    line-height: 1.55;
  }
</style>
