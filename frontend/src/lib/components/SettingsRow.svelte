<script lang="ts" module>
  /** What a row hands its control: the ids that name and describe it, and whether it is disabled. */
  export interface SettingsRowIds {
    labelId: string;
    /** Absent when the row has no hint, note or reason. */
    hintId: string | undefined;
    disabled: boolean;
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * One setting: its label and hint on the left, the control on the right; the control drops under
   * the words when the row is too narrow for both, or always with `stack`. `below` is a full-width
   * part under the row (a list of folders, an editor).
   *
   * The control snippet gets the ids of the label and the hint, so the control is named and described
   * by them (`labelledby` / `describedby` on the base controls); `labelFor` ties a single field to the
   * label instead. `note` is a line in the warning colour, read with the hint. `disabledReason` says
   * why the setting cannot be changed now: it leads the hint, and the row makes its control and its
   * `below` inert itself, so a caller that forgets to pass `disabled` on cannot leave a live control
   * under the reason. A screen reader then reads the label and the hint, the reason first, as text; the
   * control is out of the Tab order and the accessibility tree. The snippet still gets `disabled`, for
   * the control's own disabled look.
   *
   * Settings mounts well over a hundred of these: a row is one box, its words and its control. A row
   * stands on a card: the note's warning colour is text only on a surface, never on the page backdrop
   * (styles/status-surface.test.ts).
   */
  let {
    label,
    hint,
    note,
    disabledReason,
    labelFor,
    stack = false,
    testid,
    children,
    below,
  }: {
    label: string;
    hint?: string;
    note?: string;
    disabledReason?: string;
    /** Id of the control when it is a single field a `<label>` can name. */
    labelFor?: string;
    stack?: boolean;
    testid?: string;
    children?: Snippet<[SettingsRowIds]>;
    below?: Snippet<[SettingsRowIds]>;
  } = $props();

  const uid = $props.id();
  const labelId = `${uid}-label`;
  const ids = $derived<SettingsRowIds>({
    labelId,
    hintId: hint || note || disabledReason ? `${uid}-hint` : undefined,
    disabled: !!disabledReason,
  });
</script>

<div class="settings-row" class:stack data-disabled={disabledReason ? 'true' : undefined} data-testid={testid}>
  <div class="text">
    {#if labelFor}
      <label class="label" id={labelId} for={labelFor}>{label}</label>
    {:else}
      <span class="label" id={labelId}>{label}</span>
    {/if}
    {#if ids.hintId}
      <!-- Spaces between the parts: read as one description, they must not run together. -->
      <p class="hint" id={ids.hintId}>{#if disabledReason}<span class="reason">{disabledReason}</span>{' '}{/if}{#if hint}<span>{hint}</span>{' '}{/if}{#if note}<span class="note">{note}</span>{/if}</p>
    {/if}
  </div>
  {#if children}
    <div class="control" inert={!!disabledReason}>{@render children(ids)}</div>
  {/if}
  {#if below}
    <div class="below" inert={!!disabledReason}>{@render below(ids)}</div>
  {/if}
</div>

<style>
  .settings-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3) var(--space-6);
    padding: var(--space-3) 0;
  }

  /* The row before is another instance of this component: the compiler cannot see it, so it is named
     globally. */
  :global(.settings-row) + .settings-row {
    border-top: 1px solid var(--border);
  }

  .text {
    display: flex;
    flex: 1 1 22rem;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .label {
    font-size: var(--font-md);
    font-weight: 500;
    color: var(--text);
  }

  .hint {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    max-width: var(--prose-max);
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }

  .reason {
    color: var(--text-2);
  }

  .note {
    color: var(--warning);
  }

  .control {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    min-width: 0;
    max-width: 100%;
  }

  /* Under the text, as wide as the text's measure; each control at its own width from the start. A
     control with no width of its own (a slider) asks to fill (`Slider fill`) and takes the measure. */
  .stack .control {
    flex-direction: column;
    align-items: flex-start;
    justify-content: flex-start;
    flex-basis: 100%;
    max-width: var(--prose-max);
  }

  .below {
    flex-basis: 100%;
    min-width: 0;
  }
</style>
