<script lang="ts">
  /**
   * The one "working" mark. With a `label` it is a status of that name (a page or a panel that is
   * loading); without one it is decoration inside something that already says it is busy (a busy
   * button, a field with its own status). With animations off the ring stands still, its arc showing.
   */
  let {
    size = 'md',
    tone = 'accent',
    label,
  }: {
    size?: 'sm' | 'md' | 'lg';
    /** `accent`: the indicator colour, on a page or a panel. `current`: the colour of the text around it, inside a filled control. */
    tone?: 'accent' | 'current';
    label?: string;
  } = $props();
</script>

{#if label}
  <span class="spinner {size} {tone}" role="status">
    <span class="ring" aria-hidden="true"></span>
    <span class="sr-only">{label}</span>
  </span>
{:else}
  <span class="spinner {size} {tone}" aria-hidden="true"><span class="ring"></span></span>
{/if}

<style>
  .spinner {
    display: inline-flex;
    flex: none;
    width: 2rem;
    height: 2rem;
    color: var(--accent-bright);
  }

  .current {
    color: inherit;
  }

  .sm {
    width: 1.4rem;
    height: 1.4rem;
  }

  .lg {
    width: 3.2rem;
    height: 3.2rem;
  }

  .ring {
    width: 100%;
    height: 100%;
    border: 2px solid var(--hover-strong);
    border-top-color: currentColor;
    border-radius: 50%;
    animation: spinner-turn var(--motion-spin) linear infinite;
  }

  @keyframes spinner-turn {
    to {
      transform: rotate(360deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .ring {
      animation: none;
    }
  }

  :global(.no-anim) .ring {
    animation: none;
  }
</style>
