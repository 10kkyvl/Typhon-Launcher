<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';
  import { msg } from '../i18n';
  import { focusableIn, focusQuietly } from '../layers/focus';

  /**
   * A status message that belongs to a page or a section: an error, a warning, a success or a note,
   * on a plate of its own. The plate is opaque (the tone mixed into a surface, never a translucent
   * tint), so the message reads over the window backdrop in every theme; status-coloured text never
   * sits on the bare page. A failure is an alert, the rest a status. Other attributes go to the plate.
   */
  let {
    tone = 'info',
    title,
    compact = false,
    icon,
    action,
    onclose,
    children,
    ...rest
  }: Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'title' | 'children'> & {
    tone?: 'info' | 'success' | 'warning' | 'danger';
    title?: string;
    /** Inside a card or a dialog: tighter, smaller text. */
    compact?: boolean;
    /** Replaces the tone's own icon. */
    icon?: Snippet;
    action?: Snippet;
    /** Shows a close button. The page removes the notice; focus moves on to the next control. */
    onclose?: () => void;
    children?: Snippet;
  } = $props();

  let plate = $state<HTMLDivElement>();

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

<div {...rest} class="notice" class:compact data-tone={tone} role={tone === 'danger' ? 'alert' : 'status'} bind:this={plate}>
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
  <div class="body">
    {#if title}<span class="title">{title}</span>{/if}
    <div class="text">{@render children?.()}</div>
  </div>
  {#if action}
    <div class="action">{@render action()}</div>
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
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .title {
    font-weight: 600;
  }

  .text {
    overflow-wrap: anywhere;
  }

  .title + .text {
    color: var(--text-2);
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
