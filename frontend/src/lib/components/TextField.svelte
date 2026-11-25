<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import type { HTMLInputAttributes } from 'svelte/elements';

  /**
   * A single-line text field with its name, its description and its error, tied to it by id.
   *
   * Named once: by its own visible `label`, or by `labelledby`, the id of a label elsewhere on the page
   * (a settings row's). `hint` is a line under the field; `describedby` adds the page's own text;
   * `error` says what is wrong with the value. The field is described by all three, the error last, and
   * says it is invalid while there is one. An error the field mounts with is a fact of the page, read
   * with the field; one that comes later (a refused submit) is announced as it appears.
   *
   * `disabledReason` follows the settings rows: it disables the field and leads its description, on
   * screen as in the description. `disabled` alone is for a field whose place says why (a settings
   * row with its own reason). A field that only refuses edits for a while (a form waiting for its
   * answer) is `readonly`: a disabled field drops the focus it holds.
   *
   * `trailing` is one control at the field's end (a reveal or a clear button), after the field in the
   * Tab order. The field draws itself with the shared `.input` rule of global.css, the same look as the
   * raw fields not moved here yet; it adds only the red edge of an invalid value and room for the
   * trailing control. It holds no rule of its own about the value: no validation, no mask.
   */
  type Named = { label: string; labelledby?: undefined } | { labelledby: string; label?: undefined };

  let {
    value = $bindable(''),
    input = $bindable<HTMLInputElement | undefined>(undefined),
    label,
    labelledby,
    id,
    describedby,
    hint,
    error,
    type = 'text',
    autocomplete,
    inputmode,
    name,
    placeholder,
    maxlength,
    required = false,
    readonly = false,
    disabled = false,
    disabledReason,
    trailing,
    oninput,
    onchange,
    onkeydown,
    onfocus,
    onblur,
  }: Named & {
    value?: string;
    /** The field itself, for a page that moves focus to it. */
    input?: HTMLInputElement;
    /** The field's id, for a page that finds it by id; one is made when it is left out. */
    id?: string;
    /** Id(s) of the page's text that describes the field. */
    describedby?: string;
    hint?: string;
    /** What is wrong with the value; empty while nothing is. */
    error?: string;
    type?: 'text' | 'email' | 'password' | 'url' | 'tel';
    autocomplete?: HTMLInputAttributes['autocomplete'];
    inputmode?: HTMLInputAttributes['inputmode'];
    name?: string;
    placeholder?: string;
    maxlength?: number;
    required?: boolean;
    readonly?: boolean;
    disabled?: boolean;
    /** Why the field cannot be changed now: disables it and leads its description. */
    disabledReason?: string;
    trailing?: Snippet;
    oninput?: HTMLInputAttributes['oninput'];
    onchange?: HTMLInputAttributes['onchange'];
    onkeydown?: HTMLInputAttributes['onkeydown'];
    onfocus?: HTMLInputAttributes['onfocus'];
    onblur?: HTMLInputAttributes['onblur'];
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
</script>

<div class="text-field">
  {#if label}
    <label class="label" for={fieldId}>{label}</label>
  {/if}
  <div class="box">
    <input
      bind:this={input}
      bind:value
      id={fieldId}
      class="input"
      class:invalid={!!error}
      class:trailed={!!trailing}
      {type}
      {autocomplete}
      {inputmode}
      {name}
      {placeholder}
      {maxlength}
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
    />
    {#if trailing}
      <span class="trailing">{@render trailing()}</span>
    {/if}
  </div>
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
  .text-field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-width: 0;
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-2);
  }

  .box {
    position: relative;
    display: flex;
    min-width: 0;
  }

  .input.invalid {
    border-color: var(--danger);
  }

  .input.trailed {
    padding-inline-end: calc(var(--control-sm) + 0.4rem);
  }

  .trailing {
    position: absolute;
    inset-block: 0;
    inset-inline-end: 0.2rem;
    display: flex;
    align-items: center;
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

  /* On a plate of its own: the field may stand over the page backdrop, where the danger colour alone
     would not keep its contrast. */
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
