<script lang="ts">
  import { Clipboard } from '@wailsio/runtime';
  import { Copy, Link2, Unlink } from '@lucide/svelte';
  import { onDestroy, onMount } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import DiscordJoinButton from '../../lib/components/DiscordJoinButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import type { ConfirmPrompt } from '../../lib/confirm/prompts';
  import { createDiscordFlow } from '../../lib/discord/flow';
  import { formatCountdown, formatLinkCode } from '../../lib/discord/format';
  import { msg } from '../../lib/i18n';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { accountMessage } from '../../lib/services/accountMessages';
  import { discordApi, type DiscordApi } from '../../lib/services/discord';
  import { settings, updateSettingsReporting } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { handleSessionLost } from '../../lib/stores/user';
  import { focusAfterRetry } from './retryFocus';

  let { api = discordApi }: { api?: DiscordApi } = $props();

  const uid = $props.id();

  // The flow owns one session of polling; the prop is a test seam fixed at mount.
  // svelte-ignore state_referenced_locally
  const flow = createDiscordFlow(api, { onUnauthenticated: handleSessionLost });
  const view = flow.state;

  let confirming = $state<ConfirmPrompt | null>(null);

  onMount(() => void flow.load());
  onDestroy(() => flow.dispose());

  let wasLinking = false;
  $effect(() => {
    const linkedNow = $view.phase === 'linked';
    if (wasLinking && linkedNow) toast(msg('discord.linkedToast'), 'success');
    wasLinking = $view.phase === 'code';
  });

  async function copy() {
    try {
      await Clipboard.SetText(formatLinkCode($view.code));
      toast(msg('discord.codeCopied'), 'info');
    } catch {
      toast(msg('discord.codeCopyFailed'), 'danger');
    }
  }

  function askUnlink() {
    const name = $view.link?.discordUsername ?? '';
    confirming = {
      title: msg('discord.unlinkTitle'),
      text: msg('discord.unlinkText', { name }),
      confirm: msg('discord.unlinkConfirm'),
      busy: msg('discord.unlinkBusy'),
    };
  }

  async function unlink() {
    await flow.unlink();
    if ($view.phase === 'unlinked') toast(msg('discord.unlinkedToast'), 'success');
  }

  const remoteOn = $derived($settings?.discordRemoteControl ?? false);

  // The launcher registers itself with the backend before the switch turns
  // on; a failed registration leaves it off.
  function setRemote(on: boolean) {
    void updateSettingsReporting({ discordRemoteControl: on }, () => toast(msg('discord.remoteEnableFailed'), 'danger'));
  }

  const remoteDownloadsOn = $derived($settings?.discordRemoteDownloads ?? false);
  const notifyOn = $derived($settings?.discordNotifications ?? false);

  function setSetting(patch: { discordRemoteDownloads?: boolean; discordNotifications?: boolean }) {
    void updateSettingsReporting(patch, () => toast(msg('discord.settingFailed'), 'danger'));
  }

  // The error stays on screen with its Retry busy until the link answers.
  let retrying = $state(false);
  let body = $state<HTMLElement>();
  async function retry() {
    if (retrying) return;
    const from = document.activeElement;
    retrying = true;
    try {
      await flow.load();
    } finally {
      retrying = false;
    }
    await focusAfterRetry(from, body);
  }

  const steps = $derived([msg('discord.step1'), msg('discord.step2'), msg('discord.step3'), msg('discord.step4')]);
</script>

<Card title={msg('discord.cardTitle')}>
  <div class="discord" bind:this={body}>
    {#if $view.phase === 'loading' && !retrying}
      <Spinner label={msg('discord.loading')} />
    {:else if $view.phase === 'unavailable'}
      <p class="sub">{msg('discord.unavailable')}</p>
    {:else if $view.phase === 'error' || retrying}
      <Notice tone="danger" compact>
        {msg('discord.loadFailed')}
        {#snippet action()}
          <Button size="sm" busy={retrying} onclick={retry}>{msg('discord.retry')}</Button>
        {/snippet}
      </Notice>
    {:else if $view.phase === 'linked'}
      <div class="row">
        <div class="text">
          <span class="label">{msg('discord.linkedAs', { name: $view.link?.discordUsername ?? '' })}</span>
          <span class="sub">{msg('discord.linkedSub')}</span>
          {#if $view.errorCode}<div class="message"><Notice tone="danger" compact>{accountMessage($view.errorCode)}</Notice></div>{/if}
        </div>
        <Button size="sm" variant="danger" disabled={$view.busy} onclick={askUnlink}>
          <Unlink size="1.5rem" strokeWidth={1.8} />
          {msg('discord.unlinkButton')}
        </Button>
      </div>
      <div class="row">
        <div class="text">
          <span class="label" id="{uid}-remote-label">{msg('discord.remoteLabel')}</span>
          <span class="sub" id="{uid}-remote-sub">{msg('discord.remoteSub')}</span>
        </div>
        <Toggle checked={remoteOn} labelledby="{uid}-remote-label" describedby="{uid}-remote-sub" onchange={setRemote} />
      </div>
      {#if remoteOn}
        <div class="row nested">
          <div class="text">
            <span class="label" id="{uid}-remote-downloads-label">{msg('discord.remoteDownloadsLabel')}</span>
            <span class="sub" id="{uid}-remote-downloads-sub">{msg('discord.remoteDownloadsSub')}</span>
          </div>
          <Toggle
            checked={remoteDownloadsOn}
            labelledby="{uid}-remote-downloads-label"
            describedby="{uid}-remote-downloads-sub"
            onchange={(on) => setSetting({ discordRemoteDownloads: on })}
          />
        </div>
      {/if}
      <div class="row">
        <div class="text">
          <span class="label" id="{uid}-notify-label">{msg('discord.notifyLabel')}</span>
          <span class="sub" id="{uid}-notify-sub">{msg('discord.notifySub')}</span>
        </div>
        <Toggle checked={notifyOn} labelledby="{uid}-notify-label" describedby="{uid}-notify-sub" onchange={(on) => setSetting({ discordNotifications: on })} />
      </div>
    {:else if $view.phase === 'code'}
      <div class="code-block">
        <span class="sub">{msg('discord.codeLabel')}</span>
        <div class="code-row">
          <code class="code" aria-label={msg('discord.codeLabel')}>{formatLinkCode($view.code)}</code>
          <Button size="sm" onclick={copy}>
            <Copy size="1.5rem" strokeWidth={1.8} />
            {msg('discord.copyCode')}
          </Button>
        </div>
        <span class="sub">{msg('discord.expiresIn', { time: formatCountdown($view.remaining) })}</span>
      </div>
      <div class="steps">
        <span class="steps-title">{msg('discord.stepsTitle')}</span>
        <ol>
          {#each steps as step (step)}<li>{step}</li>{/each}
        </ol>
      </div>
      <p class="sub">{msg('discord.waiting')}</p>
      <div class="actions">
        <DiscordJoinButton variant="primary" />
      </div>
    {:else}
      <div class="row">
        <div class="text">
          <span class="sub">{msg('discord.intro')}</span>
          {#if $view.expired}<div class="message"><Notice tone="warning" compact>{msg('discord.expired')}</Notice></div>{/if}
          {#if $view.errorCode}<div class="message"><Notice tone="danger" compact>{accountMessage($view.errorCode)}</Notice></div>{/if}
        </div>
      </div>
      <div class="actions">
        <Button variant="primary" size="sm" disabled={$view.busy} onclick={() => void flow.start()}>
          <Link2 size="1.5rem" strokeWidth={1.8} />
          {$view.busy ? msg('discord.generating') : msg('discord.linkButton')}
        </Button>
        <DiscordJoinButton />
      </div>
    {/if}
  </div>
</Card>

{#if confirming}
  <ConfirmModal prompt={confirming} onconfirm={unlink} onclose={() => (confirming = null)} errorText={(err) => accountErrorText(err)} />
{/if}

<style>
  .discord {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-3) var(--space-6);
  }

  .nested {
    padding-left: var(--space-5);
    border-left: 2px solid var(--border);
  }

  .text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    flex: 1 1 22rem;
    overflow-wrap: anywhere;
  }

  .label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .sub {
    font-size: var(--font-xs);
    color: var(--text-3);
    line-height: 1.5;
  }

  .message {
    margin-top: var(--space-2);
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .code-block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .code-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-4);
  }

  .code {
    font-family: var(--font-mono);
    font-variant-ligatures: none;
    font-size: 3.4rem;
    font-weight: 700;
    letter-spacing: 0.18em;
    padding: var(--space-3) var(--space-5);
    border-radius: var(--radius-md);
    background: var(--surface);
    border: 1px solid var(--border);
    user-select: all;
  }

  .steps {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .steps-title {
    font-size: var(--font-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
  }

  ol {
    margin: 0;
    padding-left: 2rem;
    font-size: var(--font-sm);
    line-height: 1.7;
  }
</style>
