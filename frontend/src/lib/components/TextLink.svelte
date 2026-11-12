<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAnchorAttributes, HTMLAttributes } from 'svelte/elements';

  /**
   * An action that reads as a link inside a sentence or a list ("Forgot your password?", "Show all", a
   * breadcrumb, "Open folder"). Given `href` it is a real link (`<a href>`): it goes somewhere, and the
   * browser offers what it offers for links, and its text breaks across lines as the sentence's does.
   * Given `onclick` instead it is a `<button>`: it does something here, and only a button can be
   * disabled. A browser lays a button out as one box whatever its display, so the action form does not
   * break across lines: it is for short labels ("Reset it", "Show all", "Open folder"); a long one
   * wraps inside its own box rather than running out of the line. It keeps the font and the line height of the
   * text around it, so a line holding it is as high as one without; it is underlined, so it is told
   * from the text by more than its colour, and it shows the focus ring every control has. `tone`:
   * `accent` in running text; `muted` where the link is part of the furniture and must not pull the
   * eye (a breadcrumb, a path, a note's "show"). Muted is `--text-2` and keeps the underline: it reads
   * at 4.5:1 or more on every page surface and over the backdrop wash in every preset
   * (theme-contrast.test.ts), where `--text-3` would hold that only by hundredths. Other attributes
   * (target, rel, aria-*, data-*) go to the element.
   *
   * `aria-current` (`"page"` on a breadcrumb's link to the page shown) is passed to the element and
   * only tells assistive technology which item is the current one: it changes neither the look nor
   * what a press does, and the link stays a link that goes nowhere new. Where the current item needs
   * no action, draw it as text, not as a link.
   *
   * `start` and `end`: a mark beside the text (a chevron back, the external-link sign), passed as a
   * bare icon: the link sizes it to its text (1em), spaces it from the text and sets it on the line.
   * It is a picture; the name is the text.
   */
  type Shared = Omit<HTMLAttributes<HTMLElement>, 'class' | 'children' | 'title' | 'onclick'> & {
    children: Snippet;
    tone?: 'accent' | 'muted';
    start?: Snippet;
    end?: Snippet;
  };
  type AsLink = Shared & {
    href: string;
    target?: HTMLAnchorAttributes['target'];
    rel?: HTMLAnchorAttributes['rel'];
    onclick?: (e: MouseEvent) => void;
    disabled?: undefined;
  };
  type AsButton = Shared & {
    href?: undefined;
    target?: undefined;
    rel?: undefined;
    onclick: (e: MouseEvent) => void;
    disabled?: boolean;
  };

  let { href, disabled = false, tone = 'accent', start, end, children, ...rest }: AsLink | AsButton = $props();
</script>

{#snippet content()}
  {#if start}<span class="mark start" data-part="text-link-start" aria-hidden="true">{@render start()}</span>{/if}{@render children()}{#if end}<span class="mark end" data-part="text-link-end" aria-hidden="true">{@render end()}</span>{/if}
{/snippet}

{#if href !== undefined}
  <a {...rest} class="text-link" class:muted={tone === 'muted'} {href}>{@render content()}</a>
{:else}
  <button {...rest} class="text-link" class:muted={tone === 'muted'} type="button" {disabled}>{@render content()}</button>
{/if}

<style>
  .text-link {
    display: inline;
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    line-height: inherit;
    vertical-align: baseline;
    text-align: inherit;
    color: var(--accent-text);
    text-decoration-line: underline;
    text-decoration-thickness: var(--link-underline);
    text-decoration-color: color-mix(in srgb, currentColor 45%, transparent);
    text-underline-offset: var(--link-underline-offset);
    border-radius: var(--radius-xs);
    cursor: pointer;
    transition:
      color var(--dur) var(--ease),
      text-decoration-color var(--dur) var(--ease);
  }

  button.text-link {
    max-width: 100%;
    overflow-wrap: anywhere;
    white-space: normal;
  }

  .text-link.muted {
    color: var(--text-2);
  }

  .text-link:hover:not(:disabled) {
    color: var(--text);
    text-decoration-color: currentColor;
  }

  .mark {
    display: inline-block;
    vertical-align: var(--link-icon-drop);
  }

  .mark :global(svg) {
    display: block;
    width: var(--link-icon);
    height: var(--link-icon);
  }

  .mark.start {
    margin-inline-end: var(--link-icon-gap);
  }

  .mark.end {
    margin-inline-start: var(--link-icon-gap);
  }

  .text-link:disabled {
    opacity: 0.45;
    cursor: default;
  }
</style>
