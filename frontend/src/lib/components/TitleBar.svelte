<script lang="ts">
  import { Copy, Minus, Square, X } from '@lucide/svelte';
  import { msg } from '../i18n';
  import { captionDoubleClick, runWindowAction, windowControls, windowMaximised } from '../stores/window';
</script>

<!-- The frameless window's caption: the band drags the window, double-click toggles maximise. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="titlebar" class:native={$windowControls === 'native'} style="--wails-draggable: drag" ondblclick={(event) => captionDoubleClick(event.target)}>
  {#if $windowControls === 'native'}
    <div class="traffic-lights" aria-hidden="true"></div>
  {:else}
    <div class="window-controls" data-part="window-controls">
      <button type="button" class="wb no-drag" tabindex="-1" aria-label={msg('ui.minimize')} onclick={() => runWindowAction('minimise')}>
        <Minus aria-hidden="true" />
      </button>
      <button
        type="button"
        class="wb no-drag"
        tabindex="-1"
        aria-label={msg($windowMaximised ? 'ui.restore' : 'ui.maximize')}
        data-maximised={$windowMaximised}
        onclick={() => runWindowAction('toggleMaximise')}
      >
        {#if $windowMaximised}<Copy aria-hidden="true" />{:else}<Square aria-hidden="true" />{/if}
      </button>
      <button type="button" class="wb close no-drag" tabindex="-1" aria-label={msg('common.close')} onclick={() => runWindowAction('close')}>
        <X aria-hidden="true" />
      </button>
    </div>
  {/if}
</div>

<style>
  .titlebar {
    position: relative;
    /* Above the dialog and consent backdrops (z-index 100), so the window stays movable behind them. */
    z-index: 110;
    display: flex;
    justify-content: flex-end;
    flex-shrink: 0;
    height: var(--titlebar-h);
  }

  .titlebar.native {
    justify-content: flex-start;
  }

  .traffic-lights {
    flex: none;
    width: var(--traffic-lights-w);
  }

  .window-controls {
    display: flex;
  }

  .no-drag {
    --wails-draggable: no-drag;
  }

  .wb {
    display: grid;
    place-items: center;
    width: var(--titlebar-btn-w);
    height: 100%;
    color: var(--text-2);
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .wb :global(svg) {
    width: var(--titlebar-btn-icon);
    height: var(--titlebar-btn-icon);
    stroke-width: 1.6;
  }

  .wb:hover {
    background: var(--titlebar-btn-hover);
    color: var(--text);
  }

  .wb.close:hover {
    background: var(--titlebar-close-hover);
    color: var(--titlebar-close-text);
  }

  .wb:focus-visible {
    outline-offset: -2px;
  }
</style>
