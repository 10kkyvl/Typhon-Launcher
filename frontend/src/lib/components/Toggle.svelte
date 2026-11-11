<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';

  /**
   * A switch (WAI-ARIA switch). It has no text of its own: the row it sits in names it, through
   * `labelledby` (the id of the visible label next to it) and `describedby` (the row's description);
   * `label` is for a switch with no visible label.
   *
   * Who holds the state: with `onchange` the caller does, and the switch only asks (`onchange(next)`)
   * and shows what the caller passes back in `checked`, so a refused or failed change is never shown
   * as done; without it, `bind:checked` is written.
   */
  let {
    checked = $bindable(false),
    label,
    labelledby,
    describedby,
    disabled = false,
    onchange,
    ...rest
  }: Omit<HTMLButtonAttributes, 'class' | 'role' | 'type' | 'onclick' | 'onkeydown' | 'onchange' | 'children'> & {
    checked?: boolean;
    /** The name, when no visible label names the switch. */
    label?: string;
    /** Id of the visible label next to the switch. */
    labelledby?: string;
    /** Id of the text that says what the switch does. */
    describedby?: string;
    disabled?: boolean;
    onchange?: (checked: boolean) => void;
  } = $props();

  function toggle() {
    if (disabled) return;
    const next = !checked;
    if (onchange) onchange(next);
    else checked = next;
  }

  // A button clicks again on every repeat of a held Enter; a switch flips once per press.
  function onkeydown(event: KeyboardEvent) {
    if (event.key === 'Enter' && event.repeat) event.preventDefault();
  }
</script>

<button
  {...rest}
  class="toggle"
  class:on={checked}
  type="button"
  {disabled}
  role="switch"
  aria-checked={checked}
  aria-label={label}
  aria-labelledby={label ? undefined : labelledby}
  aria-describedby={describedby}
  onclick={toggle}
  {onkeydown}
>
  <span class="track"><span class="knob"></span></span>
</button>

<style>
  /* The button is the press target; the track drawn inside it is the switch the user sees. */
  .toggle {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    min-height: var(--toggle-target);
    border-radius: var(--radius-sm);
  }

  .toggle:focus-visible {
    outline: none;
  }

  .toggle:focus-visible .track {
    outline: 2px solid var(--accent-ring);
    outline-offset: 2px;
  }

  .toggle:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .track {
    position: relative;
    display: block;
    width: var(--toggle-w);
    height: var(--toggle-h);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--hover-strong);
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .toggle:hover:not(:disabled) .track {
    border-color: var(--text-3);
  }

  .toggle.on .track {
    border-color: var(--accent-bright);
    background: var(--accent-bright);
  }

  .toggle.on:hover:not(:disabled) .track {
    border-color: color-mix(in srgb, var(--accent-bright) 85%, var(--text));
    background: color-mix(in srgb, var(--accent-bright) 85%, var(--text));
  }

  .knob {
    position: absolute;
    top: var(--toggle-gap);
    left: var(--toggle-gap);
    width: var(--toggle-knob);
    height: var(--toggle-knob);
    border-radius: calc(var(--radius-sm) - var(--toggle-gap));
    background: var(--text-2);
    transition:
      transform var(--dur) var(--ease),
      background var(--dur) var(--ease);
  }

  .on .knob {
    background: var(--accent-on, var(--bg));
    transform: translateX(calc(var(--toggle-w) - 2px - var(--toggle-knob) - 2 * var(--toggle-gap)));
  }
</style>
