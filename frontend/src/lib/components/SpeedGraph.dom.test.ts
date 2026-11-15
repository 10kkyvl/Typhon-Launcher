import { describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import { styleOf } from '../testing/css';
import type { SpeedSeries } from '../downloads/speedHistory';
import SpeedGraph from './SpeedGraph.svelte';
import source from './SpeedGraph.svelte?raw';

const MB = 1024 ** 2;
// The drawing's own box: 240 wide, 72 high, 3 of padding at the top and the bottom.
const BOTTOM = 69;
const PLOT = 66;
const HEADROOM = 1.12;

const series = (samples: (number | null)[], moving = true): SpeedSeries => ({ samples, moving });

function draw(props: { series: SpeedSeries; window?: number; limit?: number; format?: (bytes: number) => string }) {
  const result = renderDom(SpeedGraph, props);
  const svg = result.container.querySelector('svg') as SVGSVGElement;
  return { ...result, svg };
}

/** The line's sub-paths, each a list of [x, y]. */
function segments(svg: SVGSVGElement): [number, number][][] {
  const d = svg.querySelector('path.line')?.getAttribute('d') ?? '';
  return d
    .split('M')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => part.split(/\s*L\s*/).map((point) => point.split(',').map(Number) as [number, number]));
}

const limitLine = (svg: SVGSVGElement) => svg.querySelector('line.limit');

describe('the speed graph draws a series', () => {
  it('newest on the right, one step per second of the window, zero on the floor and the peak under the headroom', () => {
    const { svg } = draw({ series: series([0, 10 * MB, 20 * MB, 30 * MB, 40 * MB]), window: 5 });
    const [line] = segments(svg);

    expect(segments(svg)).toHaveLength(1);
    expect(line.map(([x]) => x)).toEqual([0, 60, 120, 180, 240]);
    expect(line[0][1]).toBeCloseTo(BOTTOM, 1);
    expect(line[4][1]).toBeCloseTo(BOTTOM - PLOT / HEADROOM, 1);
    expect(line[2][1]).toBeCloseTo(BOTTOM - PLOT / HEADROOM / 2, 1);
  });

  it('a series shorter than the window ends at the right edge, at the window’s spacing', () => {
    const { svg } = draw({ series: series([10 * MB, 20 * MB]), window: 5 });
    expect(segments(svg)[0].map(([x]) => x)).toEqual([180, 240]);
  });

  it('draws only the window’s last seconds of a longer series', () => {
    const samples = Array.from({ length: 120 }, (_, i) => (i + 1) * MB);
    const { svg } = draw({ series: series(samples), window: 10 });
    const [line] = segments(svg);

    expect(line).toHaveLength(10);
    expect(line[0][0]).toBe(0);
    expect(line[9][0]).toBe(240);
    // The scale follows the drawn seconds: the last one (120 MB/s) is the peak.
    expect(line[9][1]).toBeCloseTo(BOTTOM - PLOT / HEADROOM, 1);
  });

  it('takes the peak and the scale over the drawn window, not a higher second that has left it', () => {
    const format = (bytes: number) => `${Math.round(bytes / MB)} MiB/s`;
    const { svg, container } = draw({ series: series([90 * MB, 10 * MB, 20 * MB, 30 * MB, 40 * MB]), window: 4, format });
    const [line] = segments(svg);

    expect(line.at(-1)?.[1]).toBeCloseTo(BOTTOM - PLOT / HEADROOM, 1);
    expect(container.querySelector('figcaption .peak')?.textContent).toContain('40 MiB/s');
    expect(container.querySelector('figcaption')?.textContent).not.toContain('90 MiB/s');
  });

  it('breaks the line at a gap instead of drawing it down to zero', () => {
    const { svg } = draw({ series: series([10 * MB, 20 * MB, null, 30 * MB, 40 * MB]), window: 5 });
    const parts = segments(svg);

    expect(parts.map((part) => part.map(([x]) => x))).toEqual([
      [0, 60],
      [180, 240],
    ]);
    for (const [, y] of parts.flat()) expect(y).toBeLessThan(BOTTOM);
  });

  it('draws no line through fewer than two points, and marks the last point with a dot', () => {
    const { svg, container } = draw({ series: series([null, null, null, null, 10 * MB]), window: 5 });
    expect(svg.querySelector('path.line')).toBeNull();
    const dot = container.querySelector<HTMLElement>('.dot');
    expect(dot?.style.left).toBe('100%');
  });

  it('a point standing alone between gaps draws no line of its own', () => {
    const { svg } = draw({ series: series([10 * MB, 20 * MB, null, 30 * MB, null]), window: 5 });
    expect(segments(svg).map((part) => part.length)).toEqual([2]);
  });

  it('with nothing in the window, draws no line and no dot', () => {
    const { svg, container } = draw({ series: series([null, null, null]), window: 5 });
    expect(svg.querySelector('path.line')).toBeNull();
    expect(container.querySelector('.dot')).toBeNull();
  });

  it('puts the dot on the last point drawn when the current second is a gap', () => {
    const { container } = draw({ series: series([10 * MB, 20 * MB, 30 * MB, null, null], false), window: 5 });
    expect(container.querySelector<HTMLElement>('.dot')?.style.left).toBe('50%');
  });
});

describe('the limit', () => {
  it('draws a dashed line at the limit only when one is set', () => {
    const none = draw({ series: series([10 * MB, 20 * MB]), window: 5 });
    expect(limitLine(none.svg)).toBeNull();
    const zero = draw({ series: series([10 * MB, 20 * MB]), window: 5, limit: 0 });
    expect(limitLine(zero.svg)).toBeNull();

    const limited = draw({ series: series([10 * MB, 20 * MB]), window: 5, limit: 15 * MB });
    const line = limitLine(limited.svg);
    expect(line).not.toBeNull();
    // Under the peak, the line sits where 15 MB/s is on a scale topped by 20 MB/s.
    expect(Number(line?.getAttribute('y1'))).toBeCloseTo(BOTTOM - (PLOT * 15) / 20 / HEADROOM, 1);
  });

  it('raises the scale to a limit above the peak, so the line stays on the drawing', () => {
    const { svg } = draw({ series: series([10 * MB, 20 * MB]), window: 5, limit: 40 * MB });
    expect(Number(limitLine(svg)?.getAttribute('y1'))).toBeCloseTo(BOTTOM - PLOT / HEADROOM, 1);
    expect(segments(svg)[0][1][1]).toBeCloseTo(BOTTOM - PLOT / HEADROOM / 2, 1);
  });
});

describe('what the graph says', () => {
  it('names the drawing by its caption: the window and the peak in it, through the formatter', () => {
    const { getByRole } = draw({ series: series([10 * MB, 41.7 * MB, null, 20 * MB]), window: 40 });
    expect(getByRole('figure', { name: 'Скорость за последние 40 сек пик 41,7 МБ/с' })).toBeTruthy();
  });

  it('says no peak with nothing in the window', () => {
    const { getByRole } = draw({ series: series([null, null]), window: 40 });
    expect(getByRole('figure').querySelector('figcaption')?.textContent?.trim()).toBe('Скорость за последние 40 сек');
  });

  it('labels the three gridlines and the peak through the caller’s formatter', () => {
    const format = (bytes: number) => `${Math.round(bytes / MB)} MiB/s`;
    const { container } = draw({ series: series([0, 100 * MB]), window: 5, format });
    const labels = [...container.querySelectorAll('.ylabel')].map((label) => label.textContent);

    expect(labels).toEqual(['84 MiB/s', '56 MiB/s', '28 MiB/s']);
    expect(container.querySelector('figcaption')?.textContent).toContain('100 MiB/s');
  });

  it('marks seconds ago along the bottom, spaced for the window', () => {
    const marks = (window: number) =>
      [...draw({ series: series([MB]), window }).container.querySelectorAll('.axis-track > span')].map((mark) => mark.textContent);
    expect(marks(40)).toEqual(['−30 сек', '−20 сек', '−10 сек']);
    expect(marks(120)).toEqual(['−90 сек', '−60 сек', '−30 сек']);
  });

  it('hides the drawing, its scale and its axis from assistive tech; the caption carries the numbers and is no live region', () => {
    const { container, svg } = draw({ series: series([10 * MB, 20 * MB]), window: 5, limit: 15 * MB });

    expect(svg.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('.gutter')?.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('.axis')?.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('.dot')?.getAttribute('aria-hidden')).toBe('true');
    expect(container.querySelector('[aria-live], [role="status"], [role="log"]')).toBeNull();
  });
});

describe('a still picture', () => {
  it('starts no timer and animates nothing', () => {
    vi.useFakeTimers();
    draw({ series: series([10 * MB, 20 * MB]), window: 5 });
    expect(vi.getTimerCount()).toBe(0);

    const css = styleOf(source, 'SpeedGraph.svelte');
    expect(css).not.toMatch(/animation|transition|@keyframes/);
    expect(source).not.toMatch(/requestAnimationFrame|setInterval|setTimeout|<animate/);
  });

  it('takes its colours from tokens and the game colour, writing none of its own', () => {
    const css = styleOf(source, 'SpeedGraph.svelte');
    expect(css).not.toMatch(/#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?|oklch)\(|(?<![-\w])(?:white|black)(?![-\w])/i);
    // The game's mark colour, not its control fill: styles/game-tone.test.ts measures it on the panel.
    expect(css).toMatch(/stroke:\s*var\(--game-accent, var\(--accent-bright\)\)/);
  });
});
