<script lang="ts" module>
  /**
   * A retry for what the notice reports (a load that failed). `run` answers whether it worked; a
   * rejection counts as a failure and is logged (the caller words its message, the notice cannot).
   * `run` may answer nothing (`undefined`) when the retry no longer matters, because what it was for
   * has gone (a dialog closed and opened again meanwhile): the notice then does nothing at all, and
   * leaves `retrying` as it is for whoever owns it now.
   * On success focus that was on Retry goes to `focus()`, else to the first control of `within()`,
   * else to the page's main heading; the getters are read after the caller has drawn what replaced
   * the notice.
   */
  export interface NoticeRetry {
    label: string;
    run: () => Promise<boolean | undefined>;
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
   * notice on screen while it is, whatever its own state says meanwhile (a load that clears its
   * failure when it starts would take the notice away mid-retry; a failure then comes back as a new
   * notice, unsaid, and in development the console names it, unless the user closed the notice or
   * what held it went too, a tab switch). Two notices that share one retry (the same failure shown in
   * two places) bind the same variable, so each stays while either runs. On
   * failure the notice stays and its message, heading and links as they are, is put into the live
   * region again, so it is said again even in the same words. A notice that does not announce becomes
   * a polite region from the first press, before the answer, so a failure is heard there too. On
   * success the caller takes the notice away as before, and focus that was on Retry moves on (see
   * NoticeRetry): with no target it stays where the notice lived, the dialog when it was in one, the
   * page's heading otherwise, and never goes into something inert. If that place has gone meanwhile,
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

  // Each failed retry puts the message into the live region again: the same nodes come in anew.
  let attempt = $state(0);
  let pressed = $state(false);
  const live = $derived(announce ? (tone === 'danger' ? 'alert' : 'status') : pressed ? 'status' : undefined);
  let alive = true;
  // The user closed it: going is not the caller dropping it.
  let closed = false;
  onDestroy(() => {
    alive = false;
  });

  async function runRetry() {
    if (!retry || retrying) return;
    const { run, focus, within } = retry;
    const button = document.activeElement instanceof HTMLElement && plate?.contains(document.activeElement) ? document.activeElement : null;
    // Where the notice lives: the dialog it is in (a layer of its own, out of the page, which is inert
    // under it), else the shell's <main> (a banner sits above it, beside the page), else the document.
    const home = plate?.closest<HTMLElement>('[role="dialog"]') ?? plate?.closest('main') ?? document.querySelector('main') ?? document.body;
    // What holds the notice: still there when the notice is gone means the caller's own condition took
    // it away; gone with it means its section went (a tab switch, the page left).
    const holder = plate?.parentElement ?? null;
    // The words at the press: a failure that comes back in other words is said by its own change.
    const said = words(message);
    retrying = true;
    pressed = true;
    let ok: boolean | undefined = false;
    try {
      ok = await run();
    } catch (err) {
      console.error('notice retry', err);
    }
    // The retry no longer matters to its caller: `retrying`, the words and the focus are not ours now.
    if (ok === undefined) return;
    // Told to the caller even when the notice went during the run: its own controls wait on it.
    retrying = false;
    await tick();
    if (ok) return moveOn(button, home, focus, within);
    if (!alive) {
      // What held it still there and the user did not close it: the caller dropped it mid-retry.
      // The mode, compared here and not through a function: `import.meta.env.DEV` is false in the
      // Wails development build (`vite build --mode development`), and the production build folds
      // this comparison to false and drops the branch (see lib/navigation/showcase.ts).
      if (import.meta.env.MODE === 'development' && !closed && holder?.isConnected) {
        console.error('Notice: removed while its retry ran, so the failure that followed was not said again. Keep the notice on screen while `retrying` is true (bind:retrying).');
      }
      return;
    }
    // The caller changed the words: they came into the live region in place and were heard once.
    if (words(message) !== said) return;
    // The same words: the message is drawn anew so they are said again. What had focus in it (a link,
    // a page-title heading) gets it back on its new twin, found by the same place in the message.
    const path = pathTo(message, document.activeElement);
    attempt += 1;
    if (!path) return;
    await tick();
    const twin = path.reduce<Element | undefined>((at, index) => at?.children[index], message);
    if (twin instanceof HTMLElement && twin.isConnected) focusQuietly(twin);
  }

  const words = (root: Element | undefined) => root?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

  // The child indexes from `root` down to `node`, or null when it is not inside.
  function pathTo(root: Element | undefined, node: Element | null): number[] | null {
    if (!root || !node || node === root || !root.contains(node)) return null;
    const path: number[] = [];
    for (let at: Element = node; at !== root; at = at.parentElement as Element) path.unshift([...(at.parentElement as Element).children].indexOf(at));
    return path;
  }

  // Focus that was on Retry would fall to the page with the notice: it goes where the caller says,
  // else stays where the notice lived (the dialog itself, or the page's heading), unless the user has
  // gone elsewhere or that place is gone. Never into something inert: a browser would refuse it and
  // leave focus on <body>.
  function moveOn(
    from: HTMLElement | null,
    home: Element | null,
    focus: NoticeRetry['focus'],
    within: NoticeRetry['within'],
  ) {
    if (!from || from.isConnected || !home?.isConnected) return;
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected) return;
    const region = within?.();
    // A dialog takes focus on its layer root, as it does when it opens with nothing to focus.
    const fallback = home.matches('[role="dialog"]') ? home.closest<HTMLElement>('[tabindex]') : home.querySelector<HTMLElement>('h1');
    const reachable = (el: HTMLElement | null | undefined) => (el?.isConnected && !el.closest('[inert]') ? el : undefined);
    const target = reachable(focus?.()) ?? reachable(region ? focusableIn(region)[0] : undefined) ?? reachable(fallback) ?? reachable(focusableIn(home)[0]);
    if (target) focusQuietly(target);
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
    closed = true;
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
  <div class="body" role={live}>
    {#key attempt}
      <div class="message" bind:this={message}>
        {#if title && level}
          <svelte:element this={`h${level}`} tabindex={level === 1 ? -1 : undefined} class="title" class:unseen>{title}</svelte:element>
        {:else if title}
          <span class="title" class:unseen>{title}</span>
        {/if}
        <div class="text" class:unseen>{@render children?.()}</div>
      </div>
    {/key}
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
