<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
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
  import { msg } from '../../lib/i18n';

  let { canonicalGameId }: { canonicalGameId: string } = $props();

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

  async function load(id: string) {
    const token = ++loadToken;
    phase = 'loading';
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
    try {
      await removeReview(canonicalGameId);
      notifyReviewsChanged();
      void load(canonicalGameId);
    } catch (err) {
      const code = err instanceof AccountError ? err.code : '';
      toast(reviewToastMessage(code, msg('reviews.composerDeleteError')), 'danger');
      throw err;
    }
  }
</script>

{#if !authed}
  <p class="prompt">{msg('reviews.composerSignInPrompt')}</p>
{:else if phase === 'loading'}
  <p class="muted">{msg('reviews.loading')}</p>
{:else if phase === 'error'}
  <p class="muted error">{msg('reviews.loadError')}</p>
  <Button size="sm" onclick={() => load(canonicalGameId)}>{msg('common.retry')}</Button>
{:else if mine}
  {#if formVisible}
    <div class="form">
      <div class="toggles">
        <button
          type="button"
          class="toggle positive"
          class:active={recommended === true}
          onclick={() => (recommended = true)}
        >
          👍 {msg('reviews.composerRecommend')}
        </button>
        <button
          type="button"
          class="toggle negative"
          class:active={recommended === false}
          onclick={() => (recommended = false)}
        >
          👎 {msg('reviews.composerNotRecommend')}
        </button>
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
        <p class="choose-hint">{msg('reviews.composerChooseFirst')}</p>
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
        <p class="hidden-note">{msg('reviews.composerHiddenNote')}</p>
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

{#if confirmDeleteOpen}
  <ConfirmModal
    prompt={{
      title: msg('reviews.composerDeleteConfirmTitle'),
      text: msg('reviews.composerDeleteConfirmText'),
      confirm: msg('reviews.composerDeleteConfirmButton'),
    }}
    onconfirm={confirmDelete}
    onclose={() => (confirmDeleteOpen = false)}
  />
{/if}

<style>
  .prompt,
  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .error {
    color: var(--danger);
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
    background: var(--accent-subtle);
    border-color: var(--accent);
    color: var(--accent-text);
  }

  .toggle.negative.active {
    background: var(--danger-subtle);
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
    border-color: var(--accent);
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

  .hint.danger,
  .counter.danger {
    color: var(--danger);
  }

  .counter {
    flex-shrink: 0;
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
    font-size: var(--font-xs);
    color: var(--warning);
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
