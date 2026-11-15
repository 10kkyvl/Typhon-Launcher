import { fireEvent, within } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import Slider from './Slider.svelte';
import source from './Slider.svelte?raw';

// The speed limit's track: 18 steps and "no limit" past them, positions 0…18.
const MAX = 18;
// The track box the tests give the slider: 180 px wide from x = 100, ten pixels a position.
const LEFT = 100;
const WIDTH = 180;
const xOf = (position: number) => LEFT + (position / MAX) * WIDTH;

type Props = {
  position: number;
  max?: number;
  label?: string;
  valueText?: string;
  page?: number;
  disabled?: boolean;
  busy?: boolean;
  startText?: string;
  endText?: string;
};

function setup(props: Props) {
  const onchange = vi.fn<(position: number) => void>();
  const oninput = vi.fn<(position: number | null) => void>();
  const all = { max: MAX, label: 'Speed limit', valueText: `${props.position}`, ...props, onchange, oninput };
  const result = renderDom(Slider, all);
  const slider = within(result.container).getByRole('slider', { name: 'Speed limit' });
  const track = result.container.querySelector<HTMLElement>('.track') as HTMLElement;
  const bar = result.container.querySelector<HTMLElement>('.bar') as HTMLElement;
  track.getBoundingClientRect = () => ({ left: LEFT, width: WIDTH, right: LEFT + WIDTH, top: 0, bottom: 4, height: 4, x: LEFT, y: 0, toJSON: () => ({}) });
  const capture = vi.fn();
  const release = vi.fn();
  bar.setPointerCapture = capture;
  bar.releasePointerCapture = release;
  bar.hasPointerCapture = () => capture.mock.calls.length > release.mock.calls.length;
  const rerender = (next: Partial<Props>) => result.rerender({ ...all, ...next });
  return { ...result, slider, bar, onchange, oninput, capture, release, rerender };
}

const where = (slider: HTMLElement) => slider.style.getPropertyValue('--at');
// Dispatched by hand: it answers whether the slider took the key (prevented its default).
const press = (slider: HTMLElement, key: string) => slider.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));

describe('Slider: what it is', () => {
  it('is a slider named by its label, with its range, the caller’s value and the caller’s words for it', () => {
    const { slider, container } = setup({ position: 7, valueText: '20 MB/s', startText: '1 MB/s', endText: 'No limit' });

    expect(slider.tabIndex).toBe(0);
    expect(slider.getAttribute('aria-valuemin')).toBe('0');
    expect(slider.getAttribute('aria-valuemax')).toBe('18');
    expect(slider.getAttribute('aria-valuenow')).toBe('7');
    expect(slider.getAttribute('aria-valuetext')).toBe('20 MB/s');
    expect(slider.getAttribute('aria-orientation')).toBe('horizontal');
    expect(slider.hasAttribute('title')).toBe(false);
    // The visible readout and the ends of the scale repeat what the slider already says.
    expect(container.querySelector('.value')?.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('.value')?.textContent).toBe('20 MB/s');
    expect(container.querySelector('.scale')?.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('.scale')?.textContent).toContain('No limit');
  });

  it('draws the thumb where the caller’s value is', () => {
    expect(where(setup({ position: 9 }).slider)).toBe('0.5');
    expect(where(setup({ position: 18 }).slider)).toBe('1');
  });
});

describe('Slider: keys', () => {
  it.each([
    ['ArrowRight', 8],
    ['ArrowUp', 8],
    ['ArrowLeft', 6],
    ['ArrowDown', 6],
    ['PageUp', 10],
    ['PageDown', 4],
    ['Home', 0],
    ['End', 18],
  ])('%s from 7 commits %i, and reports no drag', (key, expected) => {
    const { slider, onchange, oninput } = setup({ position: 7 });
    expect(press(slider, key)).toBe(false);
    expect(onchange).toHaveBeenCalledExactlyOnceWith(expected);
    expect(oninput).not.toHaveBeenCalled();
  });

  it('moves Page Up and Page Down by the step count it is given, and stops at the ends', () => {
    const { slider, onchange } = setup({ position: 16, page: 5 });
    press(slider, 'PageUp');
    press(slider, 'PageDown');
    expect(onchange.mock.calls).toEqual([[18], [11]]);
  });

  it('moves Page Up and Page Down by the step count it is given, away from the ends', () => {
    const { slider, onchange } = setup({ position: 6, page: 5 });
    press(slider, 'PageUp');
    press(slider, 'PageDown');
    expect(onchange.mock.calls).toEqual([[11], [1]]);
  });

  it('commits nothing past an end', () => {
    const start = setup({ position: 0 });
    press(start.slider, 'ArrowLeft');
    press(start.slider, 'Home');
    expect(start.onchange).not.toHaveBeenCalled();
    const end = setup({ position: 18 });
    press(end.slider, 'End');
    press(end.slider, 'PageUp');
    expect(end.onchange).not.toHaveBeenCalled();
  });

  it('leaves other keys alone', () => {
    const { slider, onchange } = setup({ position: 7 });
    expect(press(slider, 'Tab')).toBe(true);
    expect(press(slider, 'a')).toBe(true);
    expect(onchange).not.toHaveBeenCalled();
  });

  it('a refused commit leaves the thumb on the caller’s value', () => {
    const { slider, onchange } = setup({ position: 7, valueText: '20 MB/s' });
    press(slider, 'ArrowRight');
    expect(onchange).toHaveBeenCalledWith(8);
    expect(slider.getAttribute('aria-valuenow')).toBe('7');
    expect(slider.getAttribute('aria-valuetext')).toBe('20 MB/s');
    expect(where(slider)).toBe(String(7 / 18));
  });

  it('follows the caller when it takes the value', async () => {
    const { slider, rerender } = setup({ position: 7 });
    press(slider, 'ArrowRight');
    await rerender({ position: 8, valueText: '30 MB/s' });
    expect(slider.getAttribute('aria-valuenow')).toBe('8');
    expect(slider.getAttribute('aria-valuetext')).toBe('30 MB/s');
  });
});

describe('Slider: a value between steps', () => {
  it('is shown where it is, not snapped', () => {
    const { slider } = setup({ position: 5.4, valueText: '12 MB/s' });
    expect(slider.getAttribute('aria-valuenow')).toBe('5.4');
    expect(slider.getAttribute('aria-valuetext')).toBe('12 MB/s');
    expect(Number(where(slider))).toBeCloseTo(5.4 / 18, 10);
  });

  it('moves to the step on the side of the key pressed', () => {
    const { slider, onchange } = setup({ position: 5.4 });
    for (const key of ['ArrowRight', 'ArrowLeft', 'PageUp', 'PageDown']) press(slider, key);
    expect(onchange.mock.calls).toEqual([[6], [5], [8], [3]]);

    // Nearer the step above, the first step either way is still the nearest on that side.
    const near = setup({ position: 5.6 });
    for (const key of ['ArrowRight', 'ArrowLeft']) press(near.slider, key);
    expect(near.onchange.mock.calls).toEqual([[6], [5]]);
  });
});

describe('Slider: dragging', () => {
  it('reports each new step while dragging and commits once on release; the caller decides what the thumb shows', async () => {
    const { slider, bar, onchange, oninput, capture, release } = setup({ position: 7 });

    await fireEvent.pointerDown(slider, { pointerId: 3, button: 0, clientX: xOf(9) });
    expect(capture).toHaveBeenCalledWith(3);
    expect(document.activeElement).toBe(slider);
    await fireEvent.pointerMove(bar, { pointerId: 3, clientX: xOf(12) + 2 });
    await fireEvent.pointerMove(bar, { pointerId: 3, clientX: xOf(12) - 2 });
    await fireEvent.pointerMove(bar, { pointerId: 3, clientX: xOf(15) });

    expect(oninput.mock.calls).toEqual([[9], [12], [15]]);
    expect(onchange).not.toHaveBeenCalled();
    expect(slider.getAttribute('aria-valuenow')).toBe('7');

    await fireEvent.pointerUp(bar, { pointerId: 3, clientX: xOf(15) });
    expect(onchange).toHaveBeenCalledExactlyOnceWith(15);
    expect(oninput.mock.calls.at(-1)).toEqual([null]);
    expect(onchange.mock.invocationCallOrder[0]).toBeLessThan(oninput.mock.invocationCallOrder.at(-1) as number);
    expect(release).toHaveBeenCalledWith(3);
    // The caller took nothing: the thumb stays on its value.
    expect(slider.getAttribute('aria-valuenow')).toBe('7');
  });

  it('a press on the track takes the step under the pointer; past the ends it holds at them', async () => {
    const { bar, oninput, onchange } = setup({ position: 7 });
    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: LEFT - 40 });
    await fireEvent.pointerMove(bar, { pointerId: 1, clientX: LEFT + WIDTH + 40 });
    await fireEvent.pointerUp(bar, { pointerId: 1, clientX: LEFT + WIDTH + 40 });
    expect(oninput.mock.calls).toEqual([[0], [18], [null]]);
    expect(onchange).toHaveBeenCalledExactlyOnceWith(18);
  });

  // The documented caller: it shows the drag by handing each reported step back as `position`, so at
  // the release `position` already is the step under the pointer.
  it('commits a release for a caller that hands each step back, measured from where the drag began', async () => {
    const { bar, onchange, oninput, rerender } = setup({ position: 7 });
    oninput.mockImplementation((step) => void rerender({ position: step ?? 7 }));
    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(9) });
    await fireEvent.pointerMove(bar, { pointerId: 1, clientX: xOf(12) });
    await fireEvent.pointerUp(bar, { pointerId: 1, clientX: xOf(12) });
    expect(onchange).toHaveBeenCalledExactlyOnceWith(12);

    onchange.mockClear();
    await fireEvent.pointerDown(bar, { pointerId: 2, button: 0, clientX: xOf(9) });
    await fireEvent.pointerMove(bar, { pointerId: 2, clientX: xOf(7) });
    await fireEvent.pointerUp(bar, { pointerId: 2, clientX: xOf(7) });
    expect(onchange).not.toHaveBeenCalled();
  });

  it('a release on the value it started from commits nothing', async () => {
    const { bar, onchange, oninput } = setup({ position: 7 });
    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(7) });
    await fireEvent.pointerUp(bar, { pointerId: 1, clientX: xOf(7) });
    expect(onchange).not.toHaveBeenCalled();
    expect(oninput.mock.calls).toEqual([[7], [null]]);
  });

  it('a cancelled drag commits nothing and tells the caller the drag is over', async () => {
    const { bar, onchange, oninput } = setup({ position: 7 });
    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(12) });
    await fireEvent.pointerCancel(bar, { pointerId: 1 });
    await fireEvent.pointerMove(bar, { pointerId: 1, clientX: xOf(14) });
    await fireEvent.pointerUp(bar, { pointerId: 1, clientX: xOf(14) });
    expect(oninput.mock.calls).toEqual([[12], [null]]);
    expect(onchange).not.toHaveBeenCalled();
  });

  it('losing the capture ends the drag the same way', async () => {
    const { bar, onchange, oninput } = setup({ position: 7 });
    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(12) });
    await fireEvent(bar, new PointerEvent('lostpointercapture', { pointerId: 1 }));
    expect(oninput.mock.calls).toEqual([[12], [null]]);
    expect(onchange).not.toHaveBeenCalled();
  });

  it('ignores a second pointer, other buttons and a move without a press', async () => {
    const { bar, onchange, oninput } = setup({ position: 7 });
    await fireEvent.pointerMove(bar, { pointerId: 1, clientX: xOf(12) });
    await fireEvent.pointerDown(bar, { pointerId: 1, button: 2, clientX: xOf(12) });
    expect(oninput).not.toHaveBeenCalled();

    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(10) });
    await fireEvent.pointerDown(bar, { pointerId: 2, button: 0, clientX: xOf(3) });
    await fireEvent.pointerMove(bar, { pointerId: 2, clientX: xOf(4) });
    await fireEvent.pointerUp(bar, { pointerId: 2, clientX: xOf(4) });
    expect(oninput.mock.calls).toEqual([[10]]);
    expect(onchange).not.toHaveBeenCalled();
  });

  // A popover that holds the slider closes on Escape or a scroll while the pointer is still down: the
  // caller showing the drag must hear that it ended, and nothing is saved.
  it('an unmount mid-drag ends the drag: the caller is told null and nothing is committed', async () => {
    const { bar, onchange, oninput, unmount } = setup({ position: 7 });
    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(12) });
    await fireEvent.pointerMove(bar, { pointerId: 1, clientX: xOf(14) });
    unmount();
    expect(oninput.mock.calls).toEqual([[12], [14], [null]]);
    expect(onchange).not.toHaveBeenCalled();
  });

  it('an unmount without a drag tells the caller nothing', () => {
    const { oninput, onchange, unmount } = setup({ position: 7 });
    unmount();
    expect(oninput).not.toHaveBeenCalled();
    expect(onchange).not.toHaveBeenCalled();
  });

  it('keys do nothing while a drag is under way', async () => {
    const { slider, bar, onchange } = setup({ position: 7 });
    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(10) });
    press(slider, 'End');
    expect(onchange).not.toHaveBeenCalled();
  });
});

describe('Slider: disabled and busy', () => {
  for (const state of ['disabled', 'busy'] as const) {
    it(`${state}: says so, takes no key and no drag, and keeps focus when it turns ${state} under it`, async () => {
      const { slider, bar, onchange, oninput, rerender } = setup({ position: 7 });
      slider.focus();
      await rerender({ [state]: true });

      expect(document.activeElement).toBe(slider);
      expect(slider.tabIndex).toBe(0);
      expect(slider.getAttribute(state === 'disabled' ? 'aria-disabled' : 'aria-busy')).toBe('true');
      press(slider, 'ArrowRight');
      await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(12) });
      await fireEvent.pointerUp(bar, { pointerId: 1, clientX: xOf(12) });
      expect(onchange).not.toHaveBeenCalled();
      expect(oninput).not.toHaveBeenCalled();
    });
  }

  it('says neither when it is neither', () => {
    const { slider } = setup({ position: 7 });
    expect(slider.hasAttribute('aria-disabled')).toBe(false);
    expect(slider.hasAttribute('aria-busy')).toBe(false);
  });
});

describe('Slider: no motion', () => {
  it('animates nothing: the thumb and the fill jump to the value', () => {
    const css = /<style>([\s\S]*)<\/style>/.exec(source)?.[1] ?? '';
    expect(css).not.toMatch(/transition|animation|@keyframes/);
  });
});
