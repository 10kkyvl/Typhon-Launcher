<script lang="ts">
  import type { Snippet } from 'svelte';
  import { X } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';
  import { msg } from '../i18n';
  import { layer } from '../layers';

  let {
    open = $bindable(false),
    title,
    width = '46rem',
    dismissible = true,
    busy = false,
    describedby,
    initialFocus,
    returnFocus,
    onclose,
    children,
    footer,
  }: {
    open?: boolean;
    /** The visible heading and the dialog's accessible name. */
    title: string;
    width?: string;
    /** Escape, the close button and a click on the scrim close the dialog. Off while a flow must not be interrupted. */
    dismissible?: boolean;
    /** A flow is running inside (aria-busy). */
    busy?: boolean;
    /** Id of a short text in the body that describes the dialog. */
    describedby?: string;
    /** An element or a selector inside the dialog to focus on open. Default: `[data-autofocus]`, else the first control. */
    initialFocus?: HTMLElement | string;
    /** Where focus goes on close when the element that opened the dialog is gone. */
    returnFocus?: () => HTMLElement | null | undefined;
    /** The user closed the dialog (Escape, the close button, the scrim); not called when code sets `open` to false. */
    onclose?: () => void;
    children?: Snippet;
    footer?: Snippet;
  } = $props();

  const uid = $props.id();
  const titleId = `${uid}-title`;

  function dismiss() {
    if (!dismissible) return;
    open = false;
    onclose?.();
  }

  // A drag that starts in the dialog and ends on the scrim (selecting text in a field) is not a click on the scrim.
  let pressedOnScrim = false;
</script>

{#if open}
  <div
    class="overlay"
    role="presentation"
    {@attach layer({ dismiss, dismissible: () => dismissible, initialFocus: () => initialFocus, returnFocus: () => returnFocus?.() })}
    onpointerdown={(event) => (pressedOnScrim = event.target === event.currentTarget)}
    onclick={(event) => {
      if (pressedOnScrim && event.target === event.currentTarget) dismiss();
      pressedOnScrim = false;
    }}
  >
    <div
      class="modal"
      style:width
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={describedby}
      aria-busy={busy || undefined}
    >
      <div class="head">
        <h3 id={titleId}>{title}</h3>
        <span class="close" data-skip-autofocus>
          <IconButton label={msg('common.close')} size="sm" disabled={!dismissible} onclick={dismiss}>
            <X size="1.7rem" strokeWidth={1.8} />
          </IconButton>
        </span>
      </div>
      <div class="body">
        {@render children?.()}
      </div>
      {#if footer}
        <div class="foot">
          {@render footer()}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: var(--z-dialog);
    padding-top: var(--titlebar-h);
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--scrim);
    animation: scrim-in var(--dur-panel) var(--ease);
  }

  .modal {
    max-width: calc(100vw - 4.8rem);
    max-height: calc(100vh - var(--titlebar-h) - 8rem);
    display: flex;
    flex-direction: column;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-modal);
    animation: dialog-in var(--dur-panel) var(--ease);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1.8rem 1.8rem 1.6rem 2.4rem;
    border-bottom: 1px solid var(--border);
  }

  h3 {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .close {
    display: contents;
  }

  .body {
    padding: 1.6rem 2.4rem 2.4rem;
    overflow-y: auto;
    overscroll-behavior: contain;
  }

  .foot {
    display: flex;
    justify-content: flex-end;
    gap: 0.8rem;
    padding: 1.4rem 2.4rem;
    border-top: 1px solid var(--border);
  }
</style>
