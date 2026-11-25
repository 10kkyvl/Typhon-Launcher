<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import Button from './Button.svelte';

  /**
   * A path on disk with a button that chooses another: the library folder, a game's destination, an
   * image to add. The part shows the path and runs the caller's `onbrowse` (which opens the picker and
   * writes what was picked); it opens no picker itself and owns no words.
   *
   * Named once, as TextField: its own `label`, or `labelledby`, the id of a label elsewhere. Shown,
   * the path is text, not a read-only text box: a box cuts a long path to its width and a screen
   * reader reads only what fits the caret's line, while text wraps where it must (`overflow-wrap:
   * anywhere`), is read whole and can be selected and copied. The path and the button sit in a group
   * named by the label, and the button is described by the path, so a keyboard user who Tabs to it
   * hears what it would change. With no path, the caller's `placeholder` words stand in its place.
   * `editable` makes the path a text field (a destination typed as well as browsed), named and
   * described as TextField's.
   *
   * `onbrowse` may return a promise: the button is busy until it settles, a second press does
   * nothing, and focus stays on it. The caller says its own failures (a toast, `error`): a promise
   * that rejects all the same is logged, as ConfirmModal logs its action's, and the button is ready
   * again.
   *
   * `hint`, `describedby`, `error` and `disabledReason` are TextField's: described by all, the error
   * last and announced when it comes after the field was seen without one; the reason disables the
   * field and the button, leads the line under them and stays reachable on the button. `facts` is a
   * line under the path for what the caller knows about it (the drive and its free space), read
   * right after the path: Browse reads the path, the facts, the hint, the page text and the error;
   * an editable field the reason, then the same from the facts on.
   *
   * A path that cannot be changed is shown, not editable, with Browse refused by `disabledReason`.
   * While a flow runs, refuse it by `disabledReason`, not bare `disabled`: a natively disabled Browse
   * that holds focus drops it to the page body in Chromium. The shown path takes no keyboard focus;
   * a keyboard user hears it as Browse's description.
   */
  type Named = { label: string; labelledby?: undefined } | { labelledby: string; label?: undefined };

  let {
    value = $bindable(''),
    input = $bindable<HTMLInputElement | undefined>(undefined),
    label,
    labelledby,
    id,
    placeholder,
    browseLabel,
    onbrowse,
    editable = false,
    describedby,
    hint,
    error,
    disabled = false,
    disabledReason,
    facts,
  }: Named & {
    value?: string;
    /** The text field of an `editable` path, for a page that moves focus to it. */
    input?: HTMLInputElement;
    /** The text field's id when `editable`; one is made when it is left out. */
    id?: string;
    /** The words shown while there is no path ("No folder chosen"). */
    placeholder?: string;
    browseLabel: string;
    onbrowse: () => Promise<void> | void;
    /** The path is typed as well as browsed. */
    editable?: boolean;
    /** Id(s) of the page's text that describes the path. */
    describedby?: string;
    hint?: string;
    /** What is wrong with the path; empty while nothing is. */
    error?: string;
    disabled?: boolean;
    /** Why the path cannot be changed now: disables the field and the button and leads the line under them. */
    disabledReason?: string;
    facts?: Snippet;
  } = $props();

  const uid = $props.id();
  const fieldId = $derived(id ?? `${uid}-field`);
  const labelId = `${uid}-label`;
  const pathId = `${uid}-path`;
  const factsId = `${uid}-facts`;
  const reasonId = $derived(disabledReason ? `${uid}-reason` : undefined);
  const hintId = $derived(hint ? `${uid}-hint` : undefined);
  const errorId = $derived(error ? `${uid}-error` : undefined);
  const off = $derived(disabled || !!disabledReason);

  // The field's description, as TextField's with the facts after the reason: what is known about the
  // path is read next to it, as on Browse below.
  const fieldDescription = $derived([reasonId, facts ? factsId : undefined, hintId, describedby, errorId].filter(Boolean).join(' ') || undefined);
  // The button's, when the path is shown: the path and what is known about it first. The reason is
  // left out: the button says it through its own `reason`.
  const browseDescription = $derived(
    editable ? undefined : [pathId, facts ? factsId : undefined, hintId, describedby, errorId].filter(Boolean).join(' '),
  );

  let browsing = $state(false);

  // A busy, disabled or refused Button takes no press, so a second press never reaches here.
  async function browse() {
    browsing = true;
    try {
      await onbrowse();
    } catch (err) {
      console.error('path field: browse', err);
    }
    browsing = false;
  }

  // Announced from the first time the field is seen without an error.
  let announce = $state(untrack(() => !error));
  $effect(() => {
    if (!error) announce = true;
  });
</script>

<div class="path-field" role={editable ? undefined : 'group'} aria-labelledby={editable ? undefined : (labelledby ?? labelId)}>
  {#if label}
    {#if editable}
      <label class="label" id={labelId} for={fieldId}>{label}</label>
    {:else}
      <span class="label" id={labelId}>{label}</span>
    {/if}
  {/if}
  <div class="row">
    {#if editable}
      <input
        bind:this={input}
        bind:value
        id={fieldId}
        class="input sm"
        class:invalid={!!error}
        type="text"
        spellcheck="false"
        autocomplete="off"
        {placeholder}
        disabled={off}
        aria-labelledby={labelledby}
        aria-describedby={fieldDescription}
        aria-invalid={error ? 'true' : undefined}
      />
    {:else}
      <p class="path" class:empty={!value} id={pathId}>{value || placeholder || ''}</p>
    {/if}
    <span class="browse">
      <Button size="sm" busy={browsing} disabled={off} reason={disabledReason ?? ''} aria-describedby={browseDescription} onclick={browse}>
        {browseLabel}
      </Button>
    </span>
  </div>
  {#if facts}
    <div class="facts" id={factsId}>{@render facts()}</div>
  {/if}
  {#if reasonId || hintId}
    <!-- Spaces between the parts: read as one description, they must not run together. -->
    <p class="hint">{#if disabledReason}<span class="reason" id={reasonId}>{disabledReason}</span>{' '}{/if}{#if hint}<span id={hintId}>{hint}</span>{/if}</p>
  {/if}
  {#if error}
    {#key error}
      <p class="error" id={errorId} role={announce ? 'alert' : undefined}>{error}</p>
    {/key}
  {/if}
</div>

<style>
  .path-field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .path {
    flex: 1;
    min-width: 0;
    margin: 0;
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text);
    overflow-wrap: anywhere;
  }

  .path.empty {
    color: var(--text-3);
  }

  .row .input {
    flex: 1;
  }

  .input.invalid {
    border-color: var(--danger);
  }

  .browse {
    display: flex;
    flex-shrink: 0;
  }

  .facts {
    min-width: 0;
  }

  .hint {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }

  .reason {
    color: var(--text-2);
  }

  /* On a plate of its own, as TextField's: the field may stand over the page backdrop. */
  .error {
    align-self: flex-start;
    margin: 0;
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
    background: var(--surface-2);
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
