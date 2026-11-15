<script lang="ts">
  import { onDestroy } from 'svelte';
  import { fade } from 'svelte/transition';
  import { ambientState, pickAmbient } from '../stores/ambient';
  import { stageGame, stagePicture, stagePictureFailed } from '../stores/stage';
  import { libraryGames } from '../stores/library';
  import { gameArt, requestArt } from '../stores/metadata';
  import { themeApplied } from '../theme/apply';
  import { resolveColors } from '../theme/resolveColors';
  import { documentMotion, tokenMs } from '../motion/motion';
  import { BackdropPainter } from '../backdrop/painter';
  import { LookReader } from '../backdrop/lookReader';
  import { BACKDROP_H, BACKDROP_LOOK_COLORS, BACKDROP_W, backdropLook, bakeImage, loadImage, type BackdropLook } from '../backdrop/pixels';
  import { artRequest, backdropSources, playedArtRequests, rememberedGame } from '../backdrop/source';

  /**
   * The window backdrop: the art of the game the user is looking at, baked into a 64 px picture
   * (blurred by its size, dimmed and luminance-limited for the theme's text) and stretched under the
   * whole shell, the islands included. A cross-fade between games runs on those pixels in a script;
   * with animations off, reduced motion or a hidden window the picture swaps at once.
   *
   * Above it, while a screen claims the stage (stores/stage.ts): the staged game's wide art, sharp,
   * across the top of the window, solid down to the screen's header and melting into the blurred
   * picture below it. Nothing here blurs live: the soft picture is the 64 px canvas.
   */

  let canvas = $state<HTMLCanvasElement>();
  let painter = $state.raw<BackdropPainter | null>(null);
  let look = $state.raw<BackdropLook | null>(null);
  let lookKey = $state('');

  // Read from the window, not the theme file: custom CSS, a personal accent and any colour syntax are
  // resolved there.
  function readLook(): BackdropLook | null {
    const root = getComputedStyle(document.documentElement);
    const number = (name: string) => Number.parseFloat(root.getPropertyValue(name));
    return backdropLook(resolveColors({}, BACKDROP_LOOK_COLORS), number('--ambient-saturate'), number('--ambient-dim'));
  }

  const lookReader = new LookReader(readLook, (step) => requestAnimationFrame(step), (id) => cancelAnimationFrame(id), (next, key) => {
    look = next;
    lookKey = key;
  });
  onDestroy(() => lookReader.dispose());

  // themeApplied moves once the window holds a new look; the reader reads at most once a frame.
  $effect(() => {
    void $themeApplied;
    lookReader.request();
  });

  const target = $derived(
    pickAmbient($ambientState, rememberedGame($libraryGames, $gameArt), (id) => backdropSources(id, $libraryGames, $gameArt).length > 0),
  );

  $effect(() => {
    if (!look) return;
    const ids = playedArtRequests($libraryGames, $gameArt);
    for (const id of [target, $stageGame]) {
      const own = id ? artRequest(id, $libraryGames, $gameArt) : null;
      if (own && !ids.includes(own)) ids.push(own);
    }
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

  // A change of the staged game cross-fades, the old picture leaving faster than the new one arrives, so
  // another game's art never lingers under this game's header. Script-run fades ignore the no-anim class,
  // so the motion setting is asked as each fade starts: with motion off or a hidden window the swap is instant.
  function stageFade(way: 'in' | 'out'): number {
    if (!documentMotion() || document.hidden) return 0;
    return way === 'in' ? tokenMs('--motion-backdrop', 600) : tokenMs('--dur-panel', 200);
  }

  $effect(() => {
    const target = painter;
    if (!target) return;
    const ms = documentMotion() && !document.hidden ? tokenMs('--motion-backdrop', 600) : 0;
    target.show(sources, look, lookKey, ms).catch((err: unknown) => {
      console.warn('backdrop: the picture could not be drawn', err);
    });
  });
</script>

<div class="backdrop" aria-hidden="true">
  <canvas bind:this={canvas} width={BACKDROP_W} height={BACKDROP_H}></canvas>
  {#if $stagePicture}
    <div class="stage" data-game={$stagePicture.id}>
      {#key $stagePicture.src}
        {#if $stagePicture.src}
          {@const src = $stagePicture.src}
          <div class="plate" in:fade|global={{ duration: stageFade('in') }} out:fade|global={{ duration: stageFade('out') }}>
            <img {src} alt="" decoding="async" onerror={() => stagePictureFailed(src)} />
          </div>
        {/if}
      {/key}
      <div class="scrim"></div>
    </div>
  {/if}
</div>

<style>
  /* The whole window, title bar included (it paints no background of its own), at the bottom of the
     window's stacking order: the islands, the banners, the page and everything outside the shell paint
     over it. Fixed rather than absolute, so the shell does not become a containing block for its
     absolutely placed parts. */
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: var(--z-backdrop);
    overflow: hidden;
    pointer-events: none;
  }

  canvas {
    display: block;
    width: 100%;
    height: 100%;
    opacity: var(--ambient-opacity);
  }

  /* From the top of the window to the foot a screen's header ends at, and its tail below. The picture is
     solid to 78 % of that height and melts into its own blurred continuation late and quickly: a long
     fade reads as a dark band across the window at the height of the header's actions. */
  .stage {
    position: absolute;
    inset: 0 0 auto;
    height: calc(var(--stage-foot) + var(--stage-tail));
    -webkit-mask-image: linear-gradient(180deg, var(--mask-solid) 0, var(--mask-solid) 78%, var(--mask-most) 87%, var(--mask-some) 94%, transparent 100%);
    mask-image: linear-gradient(180deg, var(--mask-solid) 0, var(--mask-solid) 78%, var(--mask-most) 87%, var(--mask-some) 94%, transparent 100%);
    will-change: transform, opacity;
  }

  /* Taller than the stage: a picture's own bottom edge (a caption band, a hard crop) never shows
     through the melt. */
  .plate {
    position: absolute;
    inset: 0 0 -14%;
  }

  .plate img {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: var(--stage-focus-x) var(--stage-focus-y);
  }

  /* The top bar's row only; words on the stage bring their own plate (.stage-copy). */
  .scrim {
    position: absolute;
    inset: 0;
    background: var(--stage-scrim);
  }
</style>
