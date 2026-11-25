<script lang="ts">
  import { onMount } from 'svelte';
  import { Check, Copy } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { friendCode, rotateFriendCode } from '../../lib/services/social';
  import { toast } from '../../lib/stores/toasts';
  import { msg } from '../../lib/i18n';

  let { code = $bindable('') }: { code?: string } = $props();

  let loading = $state(!code);
  let failure = $state<unknown>(null);
  let retrying = $state(false);
  let card = $state<HTMLElement>();
  let confirmOpen = $state(false);
  let copied = $state('');

  /** Reads the code; answers whether it is there. A failure is said in the card, with Retry. */
  async function load(): Promise<boolean> {
    try {
      code = await friendCode();
      failure = null;
      return true;
    } catch (err) {
      failure = err;
      return false;
    } finally {
      loading = false;
    }
  }

  async function copy() {
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code);
      copied = code;
      toast(msg('social.copied'), 'info');
    } catch {
      toast(msg('social.copyFailed'), 'danger');
    }
  }

  const rotatePrompt = {
    title: msg('social.friendsChangeCodeTitle'),
    text: msg('social.friendsChangeCodeWarning'),
    confirm: msg('social.friendsGenerateNew'),
    busy: msg('social.friendsRotating'),
  };

  /** The confirmation runs it: a failure stays in the dialog with its words and Retry. */
  async function rotate() {
    code = await rotateFriendCode();
    toast(msg('social.friendsCodeRotated'), 'success');
  }

  onMount(() => {
    if (!code) load();
  });
</script>

<Card padding="sm">
  <div class="card" bind:this={card}>
    <div class="text">
      <span class="label">{msg('social.friendsYourCode')}</span>
      {#if failure || retrying}
        <!-- On success focus that was on Retry goes to the code, the card's first control. -->
        <Notice tone="danger" compact bind:retrying retry={{ label: msg('common.retry'), run: load, within: () => card }}>
          {accountErrorText(failure, msg('social.friendsCodeFetchFailed'))}
        </Notice>
      {:else if loading}
        <span class="code pending"><Spinner size="sm" label={msg('social.loadingEllipsis')} /></span>
      {:else if !code}
        <span class="code pending">—</span>
      {:else}
        <Tooltip text={msg('social.friendsCopyCode')}>
          <button class="code copy" type="button" onclick={copy}>{code}</button>
        </Tooltip>
      {/if}
    </div>
    <div class="actions">
      <Button size="sm" disabled={!code} onclick={copy}>
        {#if copied === code && code}
          <span class="mark"><Check size="1.5rem" strokeWidth={2} /></span>
          {msg('social.copied')}
        {:else}
          <Copy size="1.5rem" strokeWidth={1.8} />
          {msg('social.friendsCopyCode')}
        {/if}
      </Button>
      <Button variant="ghost" size="sm" disabled={!code} onclick={() => (confirmOpen = true)}>
        {msg('social.friendsGenerateNew')}
      </Button>
    </div>
  </div>
</Card>

{#if confirmOpen}
  <ConfirmModal
    prompt={rotatePrompt}
    onconfirm={rotate}
    onclose={() => (confirmOpen = false)}
    errorText={(err) => accountErrorText(err, msg('social.friendsCodeRotateFailed'))}
  />
{/if}

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

  .code.copy {
    padding: 0.2rem 0.8rem;
    margin: -0.2rem -0.8rem;
    border-radius: var(--radius-sm);
    color: var(--text);
    text-align: left;
    user-select: text;
    transition: background var(--dur) var(--ease);
  }

  .code.copy:hover {
    background: var(--hover-strong);
  }

  .mark {
    display: inline-flex;
    color: var(--success);
    animation: pop-in var(--dur-slow) var(--ease-spring);
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

</style>
