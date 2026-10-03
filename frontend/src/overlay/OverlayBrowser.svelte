<script lang="ts">
  import { ArrowLeft, ArrowRight, RotateCw, X } from '@lucide/svelte';
  import { onMount, tick } from 'svelte';
  import IconButton from '../lib/components/IconButton.svelte';
  import { errorCode, hasMessage, msg } from '../lib/i18n';
  import {
    browserArea,
    browserBack,
    browserForward,
    browserReload,
    closeBrowser,
    openBrowser,
    placeBrowser,
  } from '../lib/services/overlay';

  let { shown, onclose }: { shown: boolean; onclose: () => void } = $props();

  let address = $state('');
  let loaded = $state(false);
  let error = $state('');
  let input = $state<HTMLInputElement | undefined>(undefined);
  let viewport = $state<HTMLElement | undefined>(undefined);

  function reason(err: unknown): string {
    const code = errorCode(err);
    if (code && hasMessage(code)) return msg(code);
    return err instanceof Error ? err.message : String(err);
  }

  async function run(action: () => Promise<void>): Promise<void> {
    try {
      await action();
      error = '';
    } catch (err) {
      console.error('overlay browser', err);
      error = msg('overlay.browserError', { reason: reason(err) });
    }
  }

  async function go(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!viewport || !address.trim()) return;
    const area = browserArea(viewport);
    await run(async () => {
      address = await openBrowser(address, area);
      loaded = true;
    });
  }

  async function place(): Promise<void> {
    if (!loaded || !viewport) return;
    const area = browserArea(viewport);
    await run(() => placeBrowser(area));
  }

  async function close(): Promise<void> {
    await run(closeBrowser);
    onclose();
  }

  $effect(() => {
    if (!shown) return;
    void tick().then(place);
  });

  onMount(() => {
    input?.focus();
    const observer = new ResizeObserver(() => void place());
    if (viewport) observer.observe(viewport);
    return () => observer.disconnect();
  });
</script>

<section class="browser" aria-label={msg('overlay.browserLabel')}>
  <div class="bar">
    <IconButton label={msg('overlay.browserBack')} size="sm" disabled={!loaded} onclick={() => run(browserBack)}>
      <ArrowLeft size="1.6rem" strokeWidth={1.8} />
    </IconButton>
    <IconButton label={msg('overlay.browserForward')} size="sm" disabled={!loaded} onclick={() => run(browserForward)}>
      <ArrowRight size="1.6rem" strokeWidth={1.8} />
    </IconButton>
    <IconButton label={msg('overlay.browserReload')} size="sm" disabled={!loaded} onclick={() => run(browserReload)}>
      <RotateCw size="1.6rem" strokeWidth={1.8} />
    </IconButton>
    <form onsubmit={go}>
      <input
        bind:this={input}
        bind:value={address}
        type="text"
        spellcheck="false"
        autocomplete="off"
        maxlength="2048"
        placeholder={msg('overlay.browserPlaceholder')}
        aria-label={msg('overlay.browserAddress')}
      />
    </form>
    <IconButton label={msg('overlay.browserClose')} size="sm" onclick={close}>
      <X size="1.6rem" strokeWidth={1.8} />
    </IconButton>
  </div>
  {#if error}
    <div class="error">{error}</div>
  {/if}
  <div class="viewport" bind:this={viewport}>
    {#if !loaded}
      <p>{msg('overlay.browserEmpty')}</p>
    {/if}
  </div>
</section>

<style>
  .browser {
    position: absolute;
    top: var(--space-5);
    left: var(--space-5);
    right: calc(min(40rem, 100vw) + var(--space-5));
    bottom: var(--space-5);
    display: flex;
    flex-direction: column;
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    overflow: hidden;
  }

  .bar {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-2);
    border-bottom: 1px solid var(--border);
  }

  .bar form {
    display: flex;
    flex: 1;
    min-width: 0;
  }

  .bar input {
    flex: 1;
    min-width: 0;
    height: 3.2rem;
    padding: 0 var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    color: var(--text);
    font: inherit;
    font-size: var(--font-sm);
  }

  .bar input:focus {
    outline: none;
    border-color: var(--accent);
  }

  .error {
    padding: var(--space-2) var(--space-3);
    color: var(--danger);
    font-size: var(--font-xs);
  }

  .viewport {
    display: grid;
    flex: 1;
    min-height: 0;
    place-items: center;
    color: var(--text-3);
    font-size: var(--font-sm);
  }
</style>
