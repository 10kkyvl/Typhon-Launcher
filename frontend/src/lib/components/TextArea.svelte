<script lang="ts">
  import { untrack } from 'svelte';
  import type { HTMLTextareaAttributes } from 'svelte/elements';
  import { codePoints, counterStage, grownHeight, type CounterStage, type TextAreaCounter } from './textArea';

  /**
   * A multi-line text field: a message, a description. Named, described and refused as TextField is,
   * and drawn by the same shared `.input` rule of global.css; it is a part of its own because a
   * multi-line field is another thing: it has rows, may grow and count, takes no type or
   * autocomplete, and Enter in it makes a new line. A shortcut that sends the text (Ctrl/Cmd+Enter)
   * belongs to the caller, through `onkeydown`.
   *
   * Named once: by its own visible `label`, or by `labelledby`. `hint` is a line under the field;
   * `describedby` adds the page's own text; `error` says what is wrong with the value. The field is
   * described by all three, the error last, and says it is invalid while there is one. An error the
   * field mounts with is a fact of the page, read with the field; one that comes later is announced as
   * it appears. `disabledReason` disables the field and leads its description; a field that refuses
   * edits only while a form waits for its answer is `readonly`, so it keeps the focus it holds.
   *
   * `rows` is the height it opens at. With `grow` it follows its text down to that many rows, then
   * scrolls; without it the player can drag its height. `counter` shows the length of the text against
   * a limit the caller checks (the browser's own `maxlength` refuses typing instead). The count is on
   * screen only: assistive tech hears the caller's `limitText` once as the text comes near the limit,
   * once at it and once past it, not on every key.
   */
  type Named = { label: string; labelledby?: undefined } | { labelledby: string; label?: undefined };

  let {
    value = $bindable(''),
    textarea = $bindable<HTMLTextAreaElement | undefined>(undefined),
    label,
    labelledby,
    id,
    describedby,
    hint,
    error,
    name,
    placeholder,
    rows = 4,
    grow,
    counter,
    maxlength,
    dir = 'auto',
    required = false,
    readonly = false,
    disabled = false,
    disabledReason,
    oninput,
    onchange,
    onkeydown,
    onfocus,
    onblur,
  }: Named & {
    value?: string;
    /** The field itself, for a page that moves focus to it. */
    textarea?: HTMLTextAreaElement;
    /** The field's id, for a page that finds it by id; one is made when it is left out. */
    id?: string;
    /** Id(s) of the page's text that describes the field. */
    describedby?: string;
    hint?: string;
    /** What is wrong with the value; empty while nothing is. */
    error?: string;
    name?: string;
    placeholder?: string;
    /** The rows it opens with. */
    rows?: number;
    /** The rows it grows to with its text before it scrolls. */
    grow?: number;
    counter?: TextAreaCounter;
    /** A hard limit the browser holds typing to. */
    maxlength?: number;
    /** The direction of the text; `auto` follows the first strong letter the player types. */
    dir?: 'auto' | 'ltr' | 'rtl';
    required?: boolean;
    readonly?: boolean;
    disabled?: boolean;
    /** Why the field cannot be changed now: disables it and leads its description. */
    disabledReason?: string;
    oninput?: HTMLTextareaAttributes['oninput'];
    onchange?: HTMLTextareaAttributes['onchange'];
    onkeydown?: HTMLTextareaAttributes['onkeydown'];
    onfocus?: HTMLTextareaAttributes['onfocus'];
    onblur?: HTMLTextareaAttributes['onblur'];
  } = $props();

  const uid = $props.id();
  const fieldId = $derived(id ?? `${uid}-field`);
  const hintId = $derived(hint || disabledReason ? `${uid}-hint` : undefined);
  const errorId = $derived(error ? `${uid}-error` : undefined);
  const description = $derived([hintId, describedby, errorId].filter(Boolean).join(' ') || undefined);

  // Announced from the first time the field is seen without an error.
  let announce = $state(untrack(() => !error));
  $effect(() => {
    if (!error) announce = true;
  });

  const count = $derived(counter ? (counter.measure ?? codePoints)(value) : 0);
  const stage = $derived<CounterStage>(counter ? counterStage(count, counter.max) : 'under');

  // What the counter says, kept as it was said until the stage changes: a live region speaks on every
  // change of its text, and the count changes on every key. A stage the field opens with is not said.
  let spoken = $state('');
  let said = untrack(() => stage);
  $effect(() => {
    const now = stage;
    untrack(() => {
      if (now === said) return;
      said = now;
      spoken = now === 'under' || !counter ? '' : counter.limitText(count, counter.max);
    });
  });

  // A growing field takes its text's height. Read on every change of the value, the caller's too (a
  // field cleared after a send shrinks back).
  $effect(() => {
    void value;
    const field = textarea;
    if (!grow || !field) return;
    field.style.height = 'auto';
    const style = getComputedStyle(field);
    const px = (property: string) => parseFloat(style.getPropertyValue(property));
    const fit = grownHeight({
      scrollHeight: field.scrollHeight,
      lineHeight: px('line-height'),
      padding: px('padding-top') + px('padding-bottom'),
      border: px('border-top-width') + px('border-bottom-width'),
      rows,
      grow,
    });
    if (!fit) {
      field.style.height = '';
      return;
    }
    field.style.height = `${fit.height}px`;
    field.style.overflowY = fit.scrolls ? 'auto' : 'hidden';
  });
</script>

<!-- The rows, for the field's floor in CSS: on the wrapper, as the field itself carries no style. -->
<div class="text-area" style:--rows={rows}>
  {#if label || counter}
    <div class="head">
      {#if label}
        <label class="label" for={fieldId}>{label}</label>
      {/if}
      {#if counter}
        <span class="counter" class:over={stage === 'over'} aria-hidden="true">{counter.text(count, counter.max)}</span>
      {/if}
    </div>
  {/if}
  <textarea
    bind:this={textarea}
    bind:value
    id={fieldId}
    class="input area"
    class:invalid={!!error}
    class:grows={!!grow}
    {name}
    {placeholder}
    {rows}
    {maxlength}
    {dir}
    {required}
    {readonly}
    disabled={disabled || !!disabledReason}
    aria-labelledby={labelledby}
    aria-describedby={description}
    aria-invalid={error ? 'true' : undefined}
    {oninput}
    {onchange}
    {onkeydown}
    {onfocus}
    {onblur}
  ></textarea>
  {#if counter}
    <span class="sr-only" role="status">{spoken}</span>
  {/if}
  {#if hintId}
    <!-- Spaces between the parts: read as one description, they must not run together. -->
    <p class="hint" id={hintId}>{#if disabledReason}<span class="reason">{disabledReason}</span>{' '}{/if}{#if hint}<span>{hint}</span>{/if}</p>
  {/if}
  {#if error}
    {#key error}
      <p class="error" id={errorId} role={announce ? 'alert' : undefined}>{error}</p>
    {/key}
  {/if}
</div>

<style>
  .text-area {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
  }

  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.8rem;
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  /* The shared `.input` rule draws a one-line field: the rows set the height here, and the text gets
     room above and below. Dragged, it stops at its rows: the floor is the rows' lines, the padding
     above and below and the 1px border of `.input` (border-box). */
  .area {
    height: auto;
    min-height: calc(var(--rows) * 1lh + 1.8rem + 2px);
    padding: 0.9rem 1.2rem;
    line-height: 1.55;
    resize: vertical;
  }

  .area.grows {
    resize: none;
  }

  .input.invalid {
    border-color: var(--danger);
  }

  .counter {
    margin-inline-start: auto;
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  /* Past the limit, on a plate of its own: the field may stand over the page backdrop. */
  .counter.over {
    background: var(--surface-2);
    color: var(--danger);
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

  /* On a plate of its own, as TextField's: the danger colour alone would not keep its contrast over
     the page backdrop. */
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
