<script lang="ts">
  import { ambientState, pickAmbient } from '../stores/ambient';
  import { libraryGames } from '../stores/library';
  import { gameArt, requestArt } from '../stores/metadata';
  import { themeApplied } from '../theme/apply';
  import { resolveColors } from '../theme/resolveColors';
  import { documentMotion, tokenMs } from '../motion/plate';
  import { BackdropPainter } from '../backdrop/painter';
  import { BACKDROP_H, BACKDROP_LOOK_COLORS, BACKDROP_W, backdropLook, bakeImage, loadImage, type BackdropLook } from '../backdrop/pixels';
  import { artRequest, backdropSources, playedArtRequests, rememberedGame } from '../backdrop/source';

  /**
   * The window backdrop: the art of the game the user is looking at, baked into a 64 px picture
   * (blurred by its size, dimmed and luminance-limited for the theme's text) and stretched under the
   * whole shell, the islands included. A cross-fade between games runs on those pixels in a script;
   * with animations off, reduced motion or a hidden window the picture swaps at once.
   */

  let canvas = $state<HTMLCanvasElement>();
  let painter = $state.raw<BackdropPainter | null>(null);
  let look = $state.raw<BackdropLook | null>(null);

  // Read from the window, not the theme file: custom CSS, a personal accent and any colour syntax are
  // resolved there. themeApplied moves once the window holds a new look.
  $effect(() => {
    void $themeApplied;
    const root = getComputedStyle(document.documentElement);
    const number = (name: string) => Number.parseFloat(root.getPropertyValue(name));
    look = backdropLook(resolveColors({}, BACKDROP_LOOK_COLORS), number('--ambient-saturate'), number('--ambient-dim'));
  });

  const target = $derived(
    pickAmbient($ambientState, rememberedGame($libraryGames, $gameArt), (id) => backdropSources(id, $libraryGames, $gameArt).length > 0),
  );

  $effect(() => {
    if (!look) return;
    const ids = playedArtRequests($libraryGames, $gameArt);
    const own = target ? artRequest(target, $libraryGames, $gameArt) : null;
    if (own && !ids.includes(own)) ids.push(own);
    if (ids.length > 0) requestArt(ids);
  });

  const sources = $derived(target ? backdropSources(target, $libraryGames, $gameArt) : []);

  $effect(() => {
    const context = canvas?.getContext('2d');
    if (!context) return;
    const image = new ImageData(BACKDROP_W, BACKDROP_H);
    const next = new BackdropPainter(image.data, {
      picture: async (src, style) => {
        const img = await loadImage(src);
        return img ? bakeImage(img, style) : null;
      },
      put: () => context.putImageData(image, 0, 0),
      frame: (step) => requestAnimationFrame(step),
      cancelFrame: (id) => cancelAnimationFrame(id),
      now: () => performance.now(),
    });
    painter = next;
    return () => {
      next.dispose();
      painter = null;
    };
  });

  $effect(() => {
    const target = painter;
    if (!target) return;
    const ms = documentMotion() && !document.hidden ? tokenMs('--motion-backdrop', 600) : 0;
    target.show(sources, look, look ? JSON.stringify(look) : '', ms).catch((err: unknown) => {
      console.warn('backdrop: the picture could not be drawn', err);
    });
  });
</script>

<canvas class="backdrop" bind:this={canvas} width={BACKDROP_W} height={BACKDROP_H} aria-hidden="true"></canvas>

<style>
  /* The whole window, title bar included (it paints no background of its own), at the bottom of the
     window's stacking order: the islands, the banners, the page and everything outside the shell paint
     over it. Fixed rather than absolute, so the shell does not become a containing block for its
     absolutely placed parts. */
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: -1;
    display: block;
    width: 100%;
    height: 100%;
    opacity: var(--ambient-opacity);
    pointer-events: none;
  }
</style>
