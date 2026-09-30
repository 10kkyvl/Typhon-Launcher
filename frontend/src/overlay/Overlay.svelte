<script lang="ts">
  import { X } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import IconButton from '../lib/components/IconButton.svelte';
  import Tabs from '../lib/components/Tabs.svelte';
  import { msg } from '../lib/i18n';
  import type { ChatPeer } from '../lib/services/messaging';
  import { DEFAULT_OVERLAY_HOTKEY, hideOverlay, onOverlayEvent, onOverlayStatus, overlayView } from '../lib/services/overlay';
  import { initMessaging, openChat, unreadCount } from '../lib/stores/messaging';
  import { settings } from '../lib/stores/settings';
  import { needsSocialConsent } from '../lib/stores/social';
  import { currentUser } from '../lib/stores/user';
  import { listenFriends, overlayAccount, refreshOverlay } from './data';
  import OverlayChat from './OverlayChat.svelte';
  import OverlayFriends from './OverlayFriends.svelte';

  let { settingsFailed = false }: { settingsFailed?: boolean } = $props();

  let tab = $state('chat');
  let shown = $state(true);
  let focusTick = $state(0);
  let hideFailed = $state(false);
  let exclusive = $state(false);
  let statusHotkey = $state('');
  let panel = $state<HTMLElement | undefined>(undefined);

  const hotkey = $derived(statusHotkey || $settings?.overlayHotkey || DEFAULT_OVERLAY_HOTKEY);
  const tabs = $derived([
    { id: 'chat', label: msg('overlay.tabChat'), count: $unreadCount > 0 ? $unreadCount : undefined },
    { id: 'friends', label: msg('overlay.tabFriends') },
  ]);

  async function hide(): Promise<void> {
    try {
      await hideOverlay();
      hideFailed = false;
    } catch (err) {
      console.error('hide overlay', err);
      hideFailed = true;
    }
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Escape') return;
    event.preventDefault();
    void hide();
  }

  function onFocus(): void {
    if (!panel?.contains(document.activeElement)) panel?.focus();
  }

  function chatWith(peer: ChatPeer): void {
    openChat(peer);
    tab = 'chat';
  }

  onMount(() => {
    initMessaging({ passive: true });
    const offs = [
      onOverlayEvent('overlay:shown', (payload) => {
        shown = true;
        exclusive = payload.exclusive;
        hideFailed = false;
        focusTick += 1;
        panel?.focus();
        void refreshOverlay();
      }),
      onOverlayEvent('overlay:hidden', () => {
        shown = false;
        exclusive = false;
      }),
      onOverlayStatus((status) => {
        statusHotkey = status.hotkey;
      }),
      listenFriends(),
    ];
    panel?.focus();
    void refreshOverlay();
    overlayView()
      .then((view) => {
        if (view.visible) exclusive = view.exclusive;
      })
      .catch((err) => console.warn('overlay view failed', err));
    return () => offs.forEach((off) => off());
  });
</script>

<svelte:window onkeydown={onKeydown} onfocus={onFocus} />

<div class="overlay">
  <button class="backdrop" type="button" tabindex="-1" aria-label={msg('common.close')} onclick={hide}></button>
  <aside class="panel" bind:this={panel} tabindex="-1" aria-label={msg('overlay.panelLabel')}>
    <header class="head">
      <div class="brand">
        <strong>Typhon</strong>
        <span>{msg('overlay.hint', { key: hotkey })}</span>
      </div>
      <IconButton label={msg('common.close')} onclick={hide}><X size="1.8rem" strokeWidth={1.8} /></IconButton>
    </header>

    {#if exclusive}
      <div class="hint-note" role="note">{msg('overlay.exclusiveNotice')}</div>
    {/if}
    {#if hideFailed}
      <div class="banner">{msg('overlay.hideError')}</div>
    {/if}
    {#if settingsFailed}
      <div class="banner">{msg('state.settingsNotLoaded')}</div>
    {/if}
    {#if $overlayAccount === 'error'}
      <div class="banner">
        {msg('overlay.accountError')}
        <button type="button" onclick={() => refreshOverlay()}>{msg('common.retry')}</button>
      </div>
    {/if}

    {#if !$currentUser}
      {#if $overlayAccount === 'loading'}
        <div class="state">{msg('common.loading')}</div>
      {:else if $overlayAccount === 'signedOut'}
        <div class="state">{msg('overlay.signIn')}</div>
      {/if}
    {:else if $needsSocialConsent}
      <div class="state">{msg('overlay.consentNeeded')}</div>
    {:else}
      <div class="tabs">
        <Tabs {tabs} bind:value={tab} />
      </div>
      <div class="pane" class:off={tab !== 'chat'}>
        <OverlayChat active={shown && tab === 'chat'} {focusTick} />
      </div>
      <div class="pane" class:off={tab !== 'friends'}>
        <OverlayFriends onchat={chatWith} />
      </div>
    {/if}
  </aside>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: color-mix(in srgb, var(--bg) 55%, transparent);
  }

  .backdrop {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    background: transparent;
    cursor: default;
  }

  .panel {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    display: flex;
    flex-direction: column;
    width: min(40rem, 100vw);
    background: var(--surface);
    border-left: 1px solid var(--border-strong);
    box-shadow: var(--shadow-pop);
    outline: none;
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-4) var(--space-3);
  }

  .brand {
    display: flex;
    flex-direction: column;
    min-width: 0;
    gap: 0.2rem;
  }

  .brand strong {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .brand span {
    color: var(--text-3);
    font-size: var(--font-xs);
  }

  .tabs {
    padding: 0 var(--space-4);
  }

  .pane {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  .pane.off {
    display: none;
  }

  .hint-note {
    margin: 0 var(--space-4) var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
    font-size: var(--font-xs);
    line-height: 1.5;
  }

  .banner {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4);
    color: var(--danger);
    font-size: var(--font-xs);
  }

  .banner button {
    color: var(--accent-text);
    font: inherit;
    text-decoration: underline;
  }

  .state {
    display: grid;
    flex: 1;
    place-items: center;
    padding: var(--space-5);
    color: var(--text-3);
    font-size: var(--font-sm);
    text-align: center;
  }
</style>
