<script lang="ts">
  import { onMount } from 'svelte';
  import { Copy } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { friendCode, rotateFriendCode } from '../../lib/services/social';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';

  let {
    code = $bindable(''),
    variant = 'full',
  }: { code?: string; variant?: 'full' | 'share' } = $props();

  let loading = $state(!code);
  let rotating = $state(false);
  let confirmOpen = $state(false);

  async function load() {
    loading = true;
    try {
      code = await friendCode();
    } catch (err) {
      toast(accountErrorText(err, msg('social.friendsCodeFetchFailed')), 'danger');
    } finally {
      loading = false;
    }
  }

  async function copy() {
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code);
      toast(msg('social.copied'), 'info');
    } catch {
      toast(msg('social.copyFailed'), 'danger');
    }
  }

  async function rotate() {
    if (rotating) return;
    rotating = true;
    try {
      code = await rotateFriendCode();
      confirmOpen = false;
      toast(msg('social.friendsCodeRotated'), 'success');
    } catch (err) {
      toast(accountErrorText(err, msg('social.friendsCodeRotateFailed')), 'danger');
    } finally {
      rotating = false;
    }
  }

  onMount(() => {
    if (!code) load();
  });
</script>

{#if variant === 'share'}
  <Card title={msg('social.friendsMyCodeTitle')}>
    <div class="share">
      <p class="hint">{msg('social.friendsShareHint')}</p>
      <div class="field">
        <span class="code" class:pending={loading}>{loading ? msg('social.loadingEllipsis') : code || '—'}</span>
      </div>
      <Button variant="primary" disabled={!code} onclick={copy}>
        <Copy size="1.5rem" strokeWidth={1.8} />
        {msg('social.friendsCopyCode')}
      </Button>
    </div>
  </Card>
{:else}
  <Card padding="var(--space-4) var(--space-5)">
    <div class="card">
      <div class="text">
        <span class="label">{msg('social.friendsYourCode')}</span>
        <span class="code" class:pending={loading}>{loading ? msg('social.loadingEllipsis') : code || '—'}</span>
      </div>
      <div class="actions">
        <IconButton label={msg('social.friendsCopyCode')} size="sm" disabled={!code} onclick={copy}>
          <Copy size="1.6rem" strokeWidth={1.8} />
        </IconButton>
        <Button variant="ghost" size="sm" disabled={!code || rotating} onclick={() => (confirmOpen = true)}>
          {msg('social.friendsGenerateNew')}
        </Button>
      </div>
    </div>
  </Card>
{/if}

<Modal bind:open={confirmOpen} title={msg('social.friendsChangeCodeTitle')} width="42rem">
  <p class="warn">
    {msg('social.friendsChangeCodeWarning')}
  </p>
  {#snippet footer()}
    <Button disabled={rotating} onclick={() => (confirmOpen = false)}>{msg('common.cancel')}</Button>
    <Button variant="danger" disabled={rotating} onclick={rotate}>
      {rotating ? msg('social.friendsRotating') : msg('social.friendsGenerateNew')}
    </Button>
  {/snippet}
</Modal>

<style>
  .card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-5);
    flex-wrap: wrap;
  }

  .text {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .code {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: 0.12em;
    font-variant-numeric: tabular-nums;
  }

  .code.pending {
    font-size: var(--font-md);
    font-weight: 500;
    letter-spacing: normal;
    color: var(--text-3);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .warn {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
  }

  .share {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .hint {
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text-3);
  }

  .field {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    height: var(--control-lg);
    padding: 0 var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .field .code {
    font-size: var(--font-md);
  }
</style>
