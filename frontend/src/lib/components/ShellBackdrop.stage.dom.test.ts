import { fireEvent, render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { libraryGames } from '../stores/library';
import { gameArt } from '../stores/metadata';
import { claimStage, stageWide } from '../stores/stage';
import ShellBackdrop from './ShellBackdrop.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// jsdom has no Web Animations. A fade is given one that runs until the test ends it, as a real fade
// would run for its duration; the durations asked for are kept with the element they belong to.
interface Running {
  element: Element;
  duration: number;
  finish(): void;
}
let running: Running[] = [];
function finishAll() {
  // A transition's first animation is its delay; ending it starts the fade itself.
  for (let round = 0; round < 3; round++) {
    const now = running;
    running = [];
    for (const animation of now) animation.finish();
  }
}

const releases: (() => void)[] = [];
function stand(id: string) {
  releases.push(claimStage(id));
}

beforeEach(() => {
  running = [];
  Object.defineProperty(Element.prototype, 'animate', {
    configurable: true,
    writable: true,
    value(this: Element, _keyframes: unknown, options: { duration?: number }) {
      const animation = {
        onfinish: null as null | (() => void),
        currentTime: 0,
        playState: 'running',
        cancel() {},
      };
      running.push({ element: this, duration: options?.duration ?? 0, finish: () => animation.onfinish?.() });
      return animation;
    },
  });
  // The blurred backdrop's canvas needs a 2D context jsdom does not have; the stage does not.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  gameArt.set({
    elden: { cover: '/media/elden-cover.jpg', hero: '/media/elden-hero.jpg', heroIsPoster: false },
    celeste: { cover: '/media/celeste-cover.jpg', hero: '/media/celeste-hero.jpg', heroIsPoster: false },
    poster: { cover: '/media/poster.jpg', hero: '/media/poster.jpg', heroIsPoster: true },
    broken: { cover: '', hero: '/media/broken.jpg', heroIsPoster: false },
  });
  libraryGames.set([]);
});

afterEach(async () => {
  for (const release of releases.splice(0)) release();
  // The stage leaves under the motion setting of its own test, before the next one changes it.
  await Promise.resolve();
  await tick();
  document.documentElement.classList.remove('no-anim');
  Reflect.deleteProperty(Element.prototype, 'animate');
  vi.restoreAllMocks();
  gameArt.set({});
});

const stageOf = (root: HTMLElement) => root.querySelector<HTMLElement>('.stage');
const pictures = (root: HTMLElement) => [...root.querySelectorAll<HTMLImageElement>('.stage .plate img')];

describe('the stage in the window backdrop', () => {
  it('is not there while no screen claims it', () => {
    const { container } = render(ShellBackdrop);
    expect(stageOf(container)).toBeNull();
    expect(container.querySelector('canvas')).not.toBeNull();
  });

  it('shows the claimed game\'s wide art, one sharp picture, inside the backdrop above its canvas', async () => {
    document.documentElement.classList.add('no-anim');
    const { container } = render(ShellBackdrop);
    stand('elden');
    await tick();
    const stage = stageOf(container);
    expect(stage?.dataset.game).toBe('elden');
    expect(pictures(container).map((img) => img.getAttribute('src'))).toEqual(['/media/elden-hero.jpg']);
    const backdrop = container.querySelector('.backdrop');
    expect(backdrop?.contains(stage as HTMLElement)).toBe(true);
    expect([...(backdrop?.children ?? [])].map((child) => child.tagName.toLowerCase())).toEqual(['canvas', 'div']);
  });

  it('stays empty for a game whose hero is its poster, while the claim stands', async () => {
    document.documentElement.classList.add('no-anim');
    const { container } = render(ShellBackdrop);
    stand('poster');
    await tick();
    expect(stageOf(container)?.dataset.game).toBe('poster');
    expect(pictures(container)).toEqual([]);
  });

  it('a picture that fails to load leaves the stage empty and tells the claimer there is no wide art', async () => {
    document.documentElement.classList.add('no-anim');
    const { container } = render(ShellBackdrop);
    stand('broken');
    await tick();
    expect(get(stageWide('broken'))).toEqual({ state: 'yes', src: '/media/broken.jpg' });
    await fireEvent.error(pictures(container)[0]);
    await tick();
    expect(pictures(container)).toEqual([]);
    expect(stageOf(container)?.dataset.game).toBe('broken');
    expect(get(stageWide('broken'))).toEqual({ state: 'no' });
  });

  it('is gone once the last claim is released: it keeps no game of its own', async () => {
    document.documentElement.classList.add('no-anim');
    const { container } = render(ShellBackdrop);
    const release = claimStage('elden');
    await tick();
    expect(stageOf(container)).not.toBeNull();
    release();
    await Promise.resolve();
    await tick();
    expect(stageOf(container)).toBeNull();
  });

  it('with motion on, cross-fades to the next game: both pictures while the old one leaves faster than the new arrives', async () => {
    const { container } = render(ShellBackdrop);
    stand('elden');
    await tick();
    finishAll();
    await tick();
    stand('celeste');
    await tick();
    expect(pictures(container).map((img) => img.getAttribute('src'))).toEqual(['/media/elden-hero.jpg', '/media/celeste-hero.jpg']);
    // The delays end, the fades start: their durations are the motion tokens' (fallbacks in jsdom).
    for (const animation of running.splice(0)) animation.finish();
    const plate = (src: string) => pictures(container).find((img) => img.getAttribute('src') === src)?.closest('.plate');
    const fadeOf = (src: string) => running.find((animation) => animation.element === plate(src))?.duration;
    expect(fadeOf('/media/elden-hero.jpg')).toBe(200);
    expect(fadeOf('/media/celeste-hero.jpg')).toBe(600);
    finishAll();
    await tick();
    expect(pictures(container).map((img) => img.getAttribute('src'))).toEqual(['/media/celeste-hero.jpg']);
  });

  it('when the system asks for reduced motion, swaps to the next game at once', async () => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query.includes('prefers-reduced-motion: reduce'),
      media: query,
      onchange: null,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent: () => false,
    }));
    try {
      const { container } = render(ShellBackdrop);
      stand('elden');
      await tick();
      stand('celeste');
      await tick();
      expect(pictures(container).map((img) => img.getAttribute('src'))).toEqual(['/media/celeste-hero.jpg']);
      expect(running.filter((animation) => animation.element.matches('.plate'))).toEqual([]);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('in a hidden window, swaps to the next game at once', async () => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true });
    try {
      const { container } = render(ShellBackdrop);
      stand('elden');
      await tick();
      stand('celeste');
      await tick();
      expect(pictures(container).map((img) => img.getAttribute('src'))).toEqual(['/media/celeste-hero.jpg']);
      expect(running.filter((animation) => animation.element.matches('.plate'))).toEqual([]);
    } finally {
      Reflect.deleteProperty(document, 'hidden');
    }
  });

  it('with motion off, swaps to the next game at once', async () => {
    document.documentElement.classList.add('no-anim');
    const { container } = render(ShellBackdrop);
    stand('elden');
    await tick();
    stand('celeste');
    await tick();
    expect(pictures(container).map((img) => img.getAttribute('src'))).toEqual(['/media/celeste-hero.jpg']);
    expect(running).toEqual([]);
  });
});
