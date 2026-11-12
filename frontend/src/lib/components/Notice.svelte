<script lang="ts" module>
  /**
   * A retry for what the notice reports (a load that failed). `run` answers whether it worked; a
   * rejection counts as a failure and is logged (the caller words its message, the notice cannot).
   * On success focus that was on Retry goes to `focus()`, else to the first control of `within()`,
   * else to the page's main heading; the getters are read after the caller has drawn what replaced
   * the notice.
   */
  export interface NoticeRetry {
    label: string;
    run: () => Promise<boolean>;
    /** The label while it runs ("Checking connection…"); the label itself when absent. */
    pending?: string;
    focus?: () => HTMLElement | null | undefined;
    within?: () => HTMLElement | null | undefined;
  }
</script>

<script lang="ts">
  import { onDestroy, tick, type Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import { CircleAlert, CircleCheck, Info, RefreshCw, TriangleAlert, X } from '@lucide/svelte';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import { msg } from '../i18n';
  import { focusableIn, focusQuietly } from '../layers/focus';

  /**
   * A status message that belongs to a page or a section: an error, a warning, a success or a note,
   * on a plate of its own. The plate is opaque (the tone mixed into a surface, never a translucent
   * tint), so the message reads over the window backdrop in every theme; status-coloured text never
   * sits on the bare page. A failure is an alert, the rest a status: the title and the text are the
   * live region, the action and the close button stay outside it. Other attributes go to the plate.
   *
   * `retry` (NoticeRetry) adds a Retry after the actions, outside the live region. While it runs the
   * Retry is busy, a second press does nothing, and `retrying` (bindable) is true: the caller keeps the
   * notice on screen while it is, whatever its own state says meanwhile. On failure the notice stays
   * and its words are said again, even the same words: they come into an assistive-only copy inside
   * the live region (a region of its own on a notice that does not announce), which then stands for
   * the visible words so the page reads them once. On success the caller takes the notice away as
   * before, and focus that was on Retry moves on (see NoticeRetry); if the page has gone meanwhile,
   * nothing is focused.
   */
  let {
    tone = 'info',
    title,
    compact = false,
    announce = true,
    level,
    icon,
    action,
    onclose,
    retry,
    retrying = $bindable(false),
    children,
    ...rest
  }: Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'title' | 'children'> & {
    tone?: 'info' | 'success' | 'warning' | 'danger';
    title?: string;
    /** Inside a card or a dialog: tighter, smaller text. */
    compact?: boolean;
    /**
     * False for a standing fact of the page it is rendered with: no live region, it is read in the flow
     * of the page. A notice of something that has just happened announces by its tone.
     */
    announce?: boolean;
    /** The title is a heading of this level when the notice titles a section; plain text otherwise. */
    level?: 1 | 2 | 3 | 4 | 5 | 6;
    /** Replaces the tone's own icon. */
    icon?: Snippet;
    action?: Snippet;
    /** Shows a close button. The page removes the notice; focus moves on to the next control. */
    onclose?: () => void;
    retry?: NoticeRetry;
    /** True while `retry` runs: the caller keeps the notice on screen meanwhile. */
    retrying?: boolean;
    children?: Snippet;
  } = $props();

  let plate = $state<HTMLDivElement>();
  let message = $state<HTMLElement>();

  // The words said again after a failed retry, and whether they now stand for the visible ones.
  let again = $state('');
  let echoed = $state(false);
  let alive = true;
  let frame = 0;
  // onDestroy runs in a server render too, where no frame was ever asked for.
  onDestroy(() => {
    alive = false;
    if (frame) cancelAnimationFrame(frame);
  });

  async function runRetry() {
    if (!retry || retrying) return;
    const { run, focus, within } = retry;
    const button = document.activeElement instanceof HTMLElement && plate?.contains(document.activeElement) ? document.activeElement : null;
    // The page the notice is on: the shell's <main> (a banner sits above it, beside the page), else
    // the document.
    const page = plate?.closest('main') ?? document.querySelector('main') ?? document.body;
    retrying = true;
    // Emptied now, filled after the answer: the words come in again, so they are said again.
    again = '';
    let ok = false;
    try {
      ok = await run();
    } catch (err) {
      console.error('notice retry', err);
    }
    // Told to the caller even when the notice went during the run: its own controls wait on it.
    retrying = false;
    await tick();
    if (ok) return moveOn(button, page, focus, within);
    if (!alive) return;
    if (frame) cancelAnimationFrame(frame);
    frame = requestAnimationFrame(() => {
      frame = 0;
      if (!alive) return;
      const parts = message ? [...message.children].map((part) => part.textContent?.replace(/\s+/g, ' ').trim() ?? '') : [];
      again = parts.filter(Boolean).join(' ');
      echoed = true;
    });
  }

  // Focus that was on Retry would fall to the page with the notice: it goes where the caller says,
  // else to the page's heading, unless the user has gone elsewhere or left the page.
  function moveOn(
    from: HTMLElement | null,
    page: Element | null,
    focus: NoticeRetry['focus'],
    within: NoticeRetry['within'],
  ) {
    if (!from || from.isConnected || !page?.isConnected) return;
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected) return;
    const region = within?.();
    const target = focus?.() ?? (region ? focusableIn(region)[0] : undefined) ?? page.querySelector<HTMLElement>('h1') ?? focusableIn(page)[0];
    if (target?.isConnected) focusQuietly(target);
  }

  // The live region is the message alone. Some screen readers do not announce a polite region inserted
  // together with its content, so its words are laid out from the first frame (the plate never changes
  // size) but stay unseen, out of the accessibility tree, until the region has been on the page for a
  // frame. An alert is announced on insertion and shows its words at once.
  const polite = $derived(announce && tone !== 'danger');
  let shown = $state(false);
  const unseen = $derived(polite && !shown);
  $effect(() => {
    if (!polite) return;
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => (shown = true));
    });
    return () => cancelAnimationFrame(frame);
  });

  // The close button goes with the notice: focus would fall to the page. It moves on to what Tab
  // would reach next, else back to what came before.
  async function close() {
    const root = plate;
    const all = focusableIn(document.body);
    const mine = all.filter((el) => root?.contains(el));
    const next = all[all.indexOf(mine[mine.length - 1]) + 1] ?? all[all.indexOf(mine[0]) - 1] ?? null;
    onclose?.();
    await tick();
    const active = document.activeElement;
    if (next && next.isConnected && (!active || active === document.body || !active.isConnected)) focusQuietly(next);
  }
</script>

<div {...rest} class="notice" class:compact data-tone={tone} bind:this={plate}>
  <span class="icon" aria-hidden="true">
    {#if icon}
      {@render icon()}
    {:else if tone === 'danger'}
      <CircleAlert size="1.6rem" strokeWidth={1.8} />
    {:else if tone === 'warning'}
      <TriangleAlert size="1.6rem" strokeWidth={1.8} />
    {:else if tone === 'success'}
      <CircleCheck size="1.6rem" strokeWidth={1.8} />
    {:else}
      <Info size="1.6rem" strokeWidth={1.8} />
    {/if}
  </span>
  <div class="body" role={announce ? (tone === 'danger' ? 'alert' : 'status') : undefined}>
    <div class="message" bind:this={message} aria-hidden={echoed ? 'true' : undefined}>
      {#if title && level}
        <svelte:element this={`h${level}`} tabindex={level === 1 ? -1 : undefined} class="title" class:unseen>{title}</svelte:element>
      {:else if title}
        <span class="title" class:unseen>{title}</span>
      {/if}
      <div class="text" class:unseen>{@render children?.()}</div>
    </div>
    {#if retry}
      <span class="sr-only" data-part="notice-again" role={announce ? undefined : tone === 'danger' ? 'alert' : 'status'}>{again}</span>
    {/if}
  </div>
  {#if action || retry}
    <div class="action">
      {@render action?.()}
      {#if retry}
        <Button size="sm" busy={retrying} onclick={runRetry}>
          {#if !retrying}<RefreshCw size="1.4rem" strokeWidth={1.8} />{/if}
          {retrying && retry.pending ? retry.pending : retry.label}
        </Button>
      {/if}
    </div>
  {/if}
  {#if onclose}
    <IconButton size="sm" label={msg('common.close')} onclick={close}>
      <X size="1.4rem" strokeWidth={1.8} />
    </IconButton>
  {/if}
</div>

<style>
  .notice {
    --notice-tone: var(--accent-bright);
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    background: color-mix(in srgb, var(--notice-tone) 10%, var(--surface-2));
    border: 1px solid color-mix(in srgb, var(--notice-tone) 40%, var(--surface-2));
    border-radius: var(--radius-md);
    color: var(--text);
    font-size: var(--font-sm);
    line-height: 1.4;
  }

  .notice[data-tone='success'] {
    --notice-tone: var(--success);
  }

  .notice[data-tone='warning'] {
    --notice-tone: var(--warning);
  }

  .notice[data-tone='danger'] {
    --notice-tone: var(--danger);
  }

  .icon {
    display: flex;
    flex-shrink: 0;
    color: var(--notice-tone);
  }

  .body {
    flex: 1;
    min-width: 0;
  }

  .message {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .title {
    margin: 0;
    font-size: inherit;
    font-weight: 600;
    line-height: inherit;
    letter-spacing: inherit;
  }

  .text {
    overflow-wrap: anywhere;
  }

  .title + .text {
    color: var(--text-2);
  }

  .unseen {
    visibility: hidden;
  }

  .action {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .notice.compact {
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    font-size: var(--font-xs);
  }
</style>
