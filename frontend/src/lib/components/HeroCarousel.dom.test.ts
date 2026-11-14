import { fireEvent, screen, within } from '@testing-library/svelte';
import type { ComponentProps } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { openLayer } from '../layers';
import { ambientState } from '../stores/ambient';
import { renderDom } from '../testing/dom';
import HeroCarousel, { ADVANCE_MS, type HeroSlide } from './HeroCarousel.svelte';

type Props = ComponentProps<typeof HeroCarousel>;

const slide = (id: string, patch: Partial<HeroSlide> = {}): HeroSlide => ({
  id,
  title: `Game ${id}`,
  art: { hero: `${id}-hero.png`, cover: `${id}-cover.png` },
  line: `Played ${id}`,
  ...patch,
});

const THREE = [slide('a'), slide('b'), slide('c')];

function setup(patch: Partial<Props> = {}) {
  const props: Props = { slides: THREE, onopen: vi.fn(), onplay: vi.fn(), onstop: vi.fn(), ...patch };
  return { props, ...renderDom(HeroCarousel, props) };
}

const region = () => screen.getByRole('region', { name: msg('library.carousel.label') });
/** The title of the slide on screen: the others are hidden from assistive technology. */
const shown = () => screen.getByRole('heading', { level: 2 }).textContent?.trim();
const pickers = () => within(screen.getByRole('group', { name: msg('library.carousel.picker') })).getAllByRole('button');
const currentPicker = () => pickers().find((button) => button.getAttribute('aria-current') === 'true');
const pickLabel = (index: number, total: number, title: string) => msg('library.carousel.pick', { index: String(index), total: String(total), title });
const liveRegion = () => document.querySelector('[aria-live]') as HTMLElement;

let outside: HTMLButtonElement;

beforeEach(() => {
  outside = document.createElement('button');
  outside.textContent = 'outside';
  document.body.append(outside);
});

afterEach(() => {
  document.documentElement.classList.remove('no-anim');
  Reflect.deleteProperty(document, 'hidden');
});

describe('HeroCarousel: the timer', () => {
  it('moves to the next slide every 7 s and wraps around', async () => {
    vi.useFakeTimers();
    setup();
    expect(ADVANCE_MS).toBe(7000);
    expect(shown()).toBe('Game a');

    await vi.advanceTimersByTimeAsync(ADVANCE_MS - 1);
    expect(shown()).toBe('Game a');
    await vi.advanceTimersByTimeAsync(1);
    expect(shown()).toBe('Game b');
    await vi.advanceTimersByTimeAsync(2 * ADVANCE_MS);
    expect(shown()).toBe('Game a');
  });

  it('stops while the pointer is over it and runs on after it leaves', async () => {
    vi.useFakeTimers();
    setup();
    await vi.advanceTimersByTimeAsync(3000);

    await fireEvent.pointerEnter(region());
    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);
    expect(shown()).toBe('Game a');

    await fireEvent.pointerLeave(region());
    await vi.advanceTimersByTimeAsync(ADVANCE_MS - 3000);
    expect(shown()).toBe('Game b');
  });

  it('stops while focus is inside it and runs on after focus leaves', async () => {
    vi.useFakeTimers();
    setup();
    screen.getByRole('button', { name: msg('ui.play') }).focus();

    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);
    expect(shown()).toBe('Game a');

    outside.focus();
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(shown()).toBe('Game b');
  });

  // Switching to another app sends focusout with no relatedTarget while focus stays on the control.
  it('keeps the hold when the window loses focus with focus still inside, and runs out the time left once focus really leaves', async () => {
    vi.useFakeTimers();
    const { props, rerender } = setup();
    await vi.advanceTimersByTimeAsync(3000);
    const play = screen.getByRole('button', { name: msg('ui.play') });
    play.focus();

    play.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: null }));
    expect(document.activeElement).toBe(play);
    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);
    expect(shown()).toBe('Game a');
    await rerender({ ...props, slides: [slide('c'), slide('b'), slide('a')] });
    expect(pickers()[0].getAttribute('aria-label')).toBe(pickLabel(1, 3, 'Game a'));
    expect(document.activeElement).toBe(play);

    outside.focus();
    await vi.advanceTimersByTimeAsync(ADVANCE_MS - 3000 - 1);
    expect(shown()).toBe('Game a');
    await vi.advanceTimersByTimeAsync(1);
    expect(shown()).not.toBe('Game a');
  });

  it('stops while the document is hidden', async () => {
    vi.useFakeTimers();
    setup();
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true });
    document.dispatchEvent(new Event('visibilitychange'));

    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);
    expect(shown()).toBe('Game a');

    Reflect.deleteProperty(document, 'hidden');
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(shown()).toBe('Game b');
  });

  it('stops while a dialog is open', async () => {
    vi.useFakeTimers();
    setup();
    const dialog = document.createElement('div');
    document.body.append(dialog);
    const close = openLayer(dialog, { dismiss: () => {} });

    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);
    expect(shown()).toBe('Game a');

    close();
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(shown()).toBe('Game b');
  });

  it('does not run with motion off', async () => {
    document.documentElement.classList.add('no-anim');
    vi.useFakeTimers();
    setup();

    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);

    expect(shown()).toBe('Game a');
    expect(currentPicker()?.querySelector('.bar')).toBeNull();
  });

  it('does not run, and offers no slide controls, with one slide', async () => {
    vi.useFakeTimers();
    setup({ slides: [slide('a')] });

    await vi.advanceTimersByTimeAsync(5 * ADVANCE_MS);

    expect(shown()).toBe('Game a');
    expect(screen.queryByRole('group', { name: msg('library.carousel.picker') })).toBeNull();
  });

  it('draws its progress on the current slide control only while the timer runs', async () => {
    vi.useFakeTimers();
    setup();
    const bar = () => currentPicker()?.querySelector('.bar') as HTMLElement | null;
    expect(bar()).not.toBeNull();
    expect(pickers().filter((button) => button.querySelector('.bar'))).toHaveLength(1);

    // Drawn frame by frame: within a frame of the time run.
    const drawn = () => Number(/^scaleX\(([\d.]+)\)$/.exec(bar()?.style.transform ?? '')?.[1]);
    await vi.advanceTimersByTimeAsync(ADVANCE_MS / 2);
    expect(drawn()).toBeCloseTo(0.5, 2);

    await fireEvent.pointerEnter(region());
    const held = drawn();
    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(drawn()).toBe(held);
  });

  it('keeps the slides on screen while the pointer rests on it, and takes the new list after it leaves', async () => {
    vi.useFakeTimers();
    const { props, rerender } = setup();
    await fireEvent.pointerEnter(region());

    await rerender({ ...props, slides: [slide('x'), slide('c'), slide('a')] });
    expect(shown()).toBe('Game a');
    expect(pickers().map((button) => button.getAttribute('aria-label'))).toEqual([pickLabel(1, 3, 'Game a'), pickLabel(2, 3, 'Game b'), pickLabel(3, 3, 'Game c')]);

    await fireEvent.pointerLeave(region());
    expect(pickers().map((button) => button.getAttribute('aria-label'))).toEqual([pickLabel(1, 3, 'Game x'), pickLabel(2, 3, 'Game c'), pickLabel(3, 3, 'Game a')]);
    expect(shown()).toBe('Game a');
  });

  it('keeps the order under focus, but shows the new state of a slide (its launch in flight)', async () => {
    const { props, rerender } = setup();
    const play = screen.getByRole('button', { name: msg('ui.play') });
    play.focus();

    await rerender({ ...props, slides: [slide('c'), slide('a', { launching: true }), slide('b')] });

    expect(pickers()[0].getAttribute('aria-label')).toBe(pickLabel(1, 3, 'Game a'));
    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(play);
  });
});

describe('HeroCarousel: the slide controls', () => {
  it('names each slide control by its position and title, the current one exposed as current', () => {
    setup();
    expect(pickers().map((button) => button.getAttribute('aria-label'))).toEqual([pickLabel(1, 3, 'Game a'), pickLabel(2, 3, 'Game b'), pickLabel(3, 3, 'Game c')]);
    expect(currentPicker()).toBe(pickers()[0]);
    expect(pickers().map((button) => button.tabIndex)).toEqual([0, -1, -1]);
  });

  it('moves between them with the arrow keys, Home and End, showing each slide as it goes', async () => {
    const { user } = setup();
    pickers()[0].focus();

    await user.keyboard('{ArrowRight}');
    expect(document.activeElement).toBe(pickers()[1]);
    expect(currentPicker()).toBe(pickers()[1]);
    expect(shown()).toBe('Game b');

    await user.keyboard('{End}');
    expect(shown()).toBe('Game c');
    await user.keyboard('{ArrowRight}');
    expect(shown()).toBe('Game a');
    expect(document.activeElement).toBe(pickers()[0]);
    await user.keyboard('{ArrowLeft}');
    expect(shown()).toBe('Game c');
  });

  it('announces a slide the player picks, not one the timer moves to', async () => {
    vi.useFakeTimers();
    const { user } = setup();
    expect(liveRegion().getAttribute('aria-live')).toBe('off');

    await user.click(pickers()[2]);

    expect(shown()).toBe('Game c');
    expect(liveRegion().getAttribute('aria-live')).toBe('polite');
  });

  it('stays on the slide the player is on when the list gains a slide', async () => {
    const { user, props, rerender } = setup({ slides: [slide('a'), slide('b')] });
    await user.click(pickers()[1]);
    outside.focus();
    await fireEvent.pointerLeave(region());

    await rerender({ ...props, slides: [slide('n'), slide('a'), slide('b')] });

    expect(shown()).toBe('Game b');
    expect(currentPicker()?.getAttribute('aria-label')).toBe(pickLabel(3, 3, 'Game b'));
  });
});

describe('HeroCarousel: the game on a slide', () => {
  it('is one landmark; its title opens the game', async () => {
    const { user, props } = setup();
    await user.click(within(region()).getByRole('button', { name: 'Game a' }));
    expect(props.onopen).toHaveBeenCalledWith('a');
    expect(within(region()).getByText('Played a')).toBeTruthy();
  });

  it('Play is busy while the launch is in flight: a second press sends nothing', async () => {
    const { user, props } = setup({ slides: [slide('a', { launching: true }), slide('b')] });
    const play = screen.getByRole('button', { name: msg('ui.play') });
    expect(play.getAttribute('aria-busy')).toBe('true');
    expect(play.hasAttribute('disabled')).toBe(false);

    await user.click(play);
    play.focus();
    await user.keyboard('{Enter}');

    expect(props.onplay).not.toHaveBeenCalled();
  });

  it('a running game shows Stop, which stops it', async () => {
    const { user, props } = setup({ slides: [slide('a', { running: true }), slide('b')] });
    expect(screen.queryByRole('button', { name: msg('ui.play') })).toBeNull();

    await user.click(screen.getByRole('button', { name: msg('ui.stop') }));

    expect(props.onstop).toHaveBeenCalledWith('a');
    expect(props.onplay).not.toHaveBeenCalled();
  });

  it('Play launches an idle game', async () => {
    const { user, props } = setup();
    await user.click(screen.getByRole('button', { name: msg('ui.play') }));
    expect(props.onplay).toHaveBeenCalledWith('a');
  });

  it('feeds the window backdrop with the slide on screen when the page asks, and lets it go', async () => {
    vi.useFakeTimers();
    const { unmount } = setup({ ambient: true });
    expect(get(ambientState).claim).toBe('a');

    await vi.advanceTimersByTimeAsync(ADVANCE_MS);
    expect(get(ambientState).claim).toBe('b');

    unmount();
    expect(get(ambientState).claim).toBeNull();
  });
});
