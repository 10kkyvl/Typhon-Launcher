import { within } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import BarChartHarness from '../testing/BarChartHarness.svelte';
import { renderDom } from '../testing/dom';
import { hintOnFocus } from '../testing/hints';
import type { ChartBucket } from './barChart';

const DAYS: ChartBucket[] = [
  { key: 'mon', label: 'Пн, 2 ноября', tick: 'Пн', value: 2 },
  { key: 'tue', label: 'Вт, 3 ноября', tick: 'Вт', value: 0 },
  { key: 'wed', label: 'Ср, 4 ноября', tick: 'Ср', value: 8 },
  { key: 'thu', label: 'Чт, 5 ноября', tick: 'Чт', value: 4, current: true },
];

const said = (bucket: ChartBucket) => (bucket.value > 0 ? `${bucket.value} ч` : 'не играли');
const columns = (root: HTMLElement) => [...root.querySelectorAll<HTMLElement>('.col')];
const barHeight = (column: HTMLElement) => column.querySelector<HTMLElement>('.bar')?.style.height ?? null;
// Hidden from assistive tech by itself or by what it sits in.
const hidden = (element: Element) => element.closest('[aria-hidden="true"]') !== null;

afterEach(() => {
  document.documentElement.classList.remove('no-anim');
});

describe('BarChart', () => {
  describe('plain', () => {
    it('stands a table with the same numbers in for the drawing, which is hidden', () => {
      const { getByRole, container } = renderDom(BarChartHarness, { buckets: DAYS });
      const table = getByRole('table', { name: 'Время по дням' });
      const rows = within(table).getAllByRole('row').slice(1);
      expect(rows.map((row) => within(row).getAllByRole('rowheader').map((cell) => cell.textContent))).toEqual(DAYS.map((d) => [d.label]));
      expect(rows.map((row) => within(row).getByRole('cell').textContent)).toEqual(DAYS.map(said));
      expect(within(table).getAllByRole('columnheader').map((cell) => cell.textContent)).toEqual(['Период', 'Значение']);
      expect(columns(container).length).toBe(DAYS.length);
      expect(columns(container).every(hidden)).toBe(true);
      expect(container.querySelector('[title]')).toBeNull();
    });

    it('keeps the slot of a zero bucket and draws no bar in it', () => {
      const { container } = renderDom(BarChartHarness, { buckets: DAYS });
      expect(columns(container).map(barHeight)).toEqual(['25%', null, '100%', '50%']);
      expect(columns(container).map((column) => column.querySelector('.tick')?.textContent)).toEqual(['Пн', 'Вт', 'Ср', 'Чт']);
    });

    it('shows the value of every bar as a label, without hover', () => {
      const { container } = renderDom(BarChartHarness, { buckets: DAYS });
      expect(columns(container).map((column) => column.querySelector('.value')?.textContent ?? null)).toEqual(['2 ч', null, '8 ч', '4 ч']);
    });

    it('marks the bucket now is in', () => {
      const { container } = renderDom(BarChartHarness, { buckets: DAYS });
      expect(columns(container).map((column) => column.classList.contains('current'))).toEqual([false, false, false, true]);
    });

    it('measures the bars against the scale it is given, and draws its ticks', () => {
      const { container } = renderDom(BarChartHarness, { buckets: [...DAYS, { key: 'fri', label: 'Пт', value: 40 }], scale: { max: 16, step: 4 } });
      expect(columns(container).map(barHeight)).toEqual(['12.5%', null, '50%', '25%', '100%']);
      expect([...container.querySelectorAll('.axis .mark')].map((mark) => mark.textContent)).toEqual(['0', '4 ч', '8 ч', '12 ч', '16 ч']);
      expect(hidden(container.querySelector('.axis') as Element)).toBe(true);
    });

    it('keeps one huge bar from hiding the others: each still has a bar', () => {
      const buckets: ChartBucket[] = [
        { key: 'a', label: 'A', value: 100000 },
        { key: 'b', label: 'B', value: 1 },
      ];
      const { container } = renderDom(BarChartHarness, { buckets });
      expect(columns(container).map((column) => column.querySelector('.bar') !== null)).toEqual([true, true]);
      expect(barHeight(columns(container)[0])).toBe('100%');
    });

    for (const [name, buckets] of [
      ['no buckets', []],
      ['nothing above zero', DAYS.map((d) => ({ ...d, value: 0 }))],
    ] as const) {
      it(`shows the caller's empty state with ${name}`, () => {
        const { getByTestId, queryByRole, container } = renderDom(BarChartHarness, { buckets: [...buckets] });
        expect(getByTestId('empty').textContent).toBe('За неделю не играли');
        expect(queryByRole('table')).toBeNull();
        expect(columns(container)).toEqual([]);
      });
    }
  });

  describe('with a selection', () => {
    const radios = (root: HTMLElement) => within(root).getAllByRole('radio');

    it('is a named radio group whose radios carry the numbers, one Tab stop on the chosen one', () => {
      const { getByRole, container, queryByRole } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'wed' });
      expect(getByRole('radiogroup', { name: 'Недели' })).toBeTruthy();
      expect(radios(container).map((radio) => radio.getAttribute('aria-label'))).toEqual(DAYS.map((d) => `${d.label}: ${said(d)}`));
      expect(radios(container).map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'true', 'false']);
      expect(radios(container).map((radio) => radio.tabIndex)).toEqual([-1, -1, 0, -1]);
      // The radios are the list with the same numbers: a table beside them would say each twice.
      expect(queryByRole('table')).toBeNull();
      expect(container.querySelector('[title]')).toBeNull();
    });

    it('moves the choice and the focus with the arrows, Home and End, and stops at the ends', async () => {
      const { container, getByTestId, user } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'wed' });
      const all = radios(container);
      const at = () => all.indexOf(document.activeElement as HTMLElement);
      const chosen = () => getByTestId('selected').textContent;

      await user.tab();
      expect(at()).toBe(2);
      await user.keyboard('{ArrowRight}');
      expect([at(), chosen()]).toEqual([3, 'thu']);
      await user.keyboard('{ArrowRight}');
      expect([at(), chosen()]).toEqual([3, 'thu']);
      await user.keyboard('{ArrowLeft}');
      expect([at(), chosen()]).toEqual([2, 'wed']);
      await user.keyboard('{Home}');
      expect([at(), chosen()]).toEqual([0, 'mon']);
      await user.keyboard('{ArrowLeft}');
      expect([at(), chosen()]).toEqual([0, 'mon']);
      await user.keyboard('{End}');
      expect([at(), chosen()]).toEqual([3, 'thu']);
      expect(all.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'false', 'true']);
      expect(all.map((radio) => radio.tabIndex)).toEqual([-1, -1, -1, 0]);
    });

    it('chooses a pressed bar, a zero one too', async () => {
      const { container, getByTestId, user } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'wed' });
      await user.click(radios(container)[1]);
      expect(getByTestId('selected').textContent).toBe('tue');
      expect(radios(container)[1].getAttribute('aria-checked')).toBe('true');
    });

    it('keeps a Tab stop when the choice is no bucket: the current one', () => {
      const { container } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'gone' });
      expect(radios(container).map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'false', 'false']);
      expect(radios(container).map((radio) => radio.tabIndex)).toEqual([-1, -1, -1, 0]);
    });

    it('shows the value of a focused bar in a hint', async () => {
      const { container } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'mon' });
      expect(await hintOnFocus(radios(container)[0])).toBe('Пн, 2 ноября: 2 ч');
    });

    it('labels only the chosen bar when asked to, for a chart of many bars', async () => {
      const { container, user } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'wed', values: 'selected' });
      const labels = () => columns(container).map((column) => column.querySelector('.value')?.textContent ?? null);
      expect(labels()).toEqual([null, null, '8 ч', null]);
      await user.click(radios(container)[0]);
      expect(labels()).toEqual(['2 ч', null, null, null]);
    });

    it('is a radio group before anything is chosen, the Tab stop on the current bar', () => {
      const { getByRole, container, queryByRole } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true });
      expect(getByRole('radiogroup', { name: 'Недели' })).toBeTruthy();
      expect(radios(container).map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'false', 'false']);
      expect(radios(container).map((radio) => radio.tabIndex)).toEqual([-1, -1, -1, 0]);
      expect(queryByRole('table')).toBeNull();
    });

    it('stays a radio group, the focused radio kept, when the page clears the choice', async () => {
      const { getByRole, container, rerender, user } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'wed' });
      await user.tab();
      const focused = document.activeElement;
      expect(focused).toBe(radios(container)[2]);
      await rerender({ start: undefined });
      expect(getByRole('radiogroup')).toBeTruthy();
      expect(radios(container).map((radio) => radio.getAttribute('aria-checked'))).toEqual(['false', 'false', 'false', 'false']);
      expect(document.activeElement).toBe(focused);
      expect((focused as HTMLElement).isConnected).toBe(true);
    });

    describe('as the buckets change', () => {
      const aria = () => document.activeElement?.getAttribute('aria-label');

      it('hands focus to the bar now in the place of a focused one that leaves', async () => {
        const { rerender, user } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'wed' });
        await user.tab();
        expect(aria()).toBe('Ср, 4 ноября: 8 ч');
        await rerender({ buckets: DAYS.filter((d) => d.key !== 'wed') });
        expect(aria()).toBe('Чт, 5 ноября: 4 ч');
        expect((document.activeElement as HTMLElement).tabIndex).toBe(0);
      });

      it('hands focus to the neighbour when the last one leaves', async () => {
        const { rerender, user } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'thu' });
        await user.tab();
        await rerender({ buckets: DAYS.slice(0, 3) });
        expect(aria()).toBe('Ср, 4 ноября: 8 ч');
      });

      it('keeps the Tab stop on the bar focus was handed to, by its key, when one is added before it', async () => {
        const { container, rerender, user } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'mon' });
        await user.tab();
        await rerender({ buckets: DAYS.slice(1) });
        expect(aria()).toBe('Вт, 3 ноября: не играли');
        await user.tab();
        await rerender({ buckets: [{ key: 'sun', label: 'Вс, 1 ноября', value: 1 }, ...DAYS.slice(1)] });
        expect(radios(container).filter((radio) => radio.tabIndex === 0).map((radio) => radio.getAttribute('aria-label'))).toEqual(['Вт, 3 ноября: не играли']);
      });

      it('leaves focus alone when it was not on a bar', async () => {
        const { rerender, user } = renderDom(BarChartHarness, { buckets: DAYS, selectable: true, start: 'wed' });
        const outside = document.createElement('button');
        document.body.append(outside);
        await user.tab();
        outside.focus();
        await rerender({ buckets: DAYS.filter((d) => d.key !== 'wed') });
        expect(document.activeElement).toBe(outside);
        outside.remove();
      });
    });

    // A bar above the scale is drawn to the top and says what it holds, not what was drawn.
    it('clamps a value over the scale to the top and labels it with the real value', () => {
      const buckets: ChartBucket[] = [...DAYS, { key: 'fri', label: 'Пт, 6 ноября', value: 20 }];
      const { container } = renderDom(BarChartHarness, { buckets, selectable: true, start: 'fri', scale: { max: 8, step: 2 }, values: 'selected' });
      const last = columns(container).at(-1) as HTMLElement;
      expect(barHeight(last)).toBe('100%');
      expect(last.querySelector('.value')?.textContent).toBe('20 ч');
      expect(radios(container).at(-1)?.getAttribute('aria-label')).toBe('Пт, 6 ноября: 20 ч');
    });
  });

  describe('the values it is given', () => {
    it('stands in the real values, not the drawn ones, for bars clamped to the scale', () => {
      const buckets: ChartBucket[] = [
        { key: 'a', label: 'A', value: 4 },
        { key: 'b', label: 'B', value: 20 },
      ];
      const { getByRole, container } = renderDom(BarChartHarness, { buckets, scale: { max: 8, step: 2 } });
      expect(columns(container).map(barHeight)).toEqual(['50%', '100%']);
      const cells = within(getByRole('table')).getAllByRole('cell').map((cell) => cell.textContent);
      expect(cells).toEqual(['4 ч', '20 ч']);
    });

    it('scales values under 1 to the largest of them', () => {
      const buckets: ChartBucket[] = [
        { key: 'a', label: 'A', value: 0.5 },
        { key: 'b', label: 'B', value: 0.25 },
      ];
      const { container } = renderDom(BarChartHarness, { buckets });
      expect(columns(container).map(barHeight)).toEqual(['100%', '50%']);
    });

    it('treats a value that is not a number, below zero or infinite as no value: bar, label, table and radio', () => {
      const buckets: ChartBucket[] = [
        { key: 'nan', label: 'NaN', value: Number.NaN },
        { key: 'neg', label: 'Neg', value: -3 },
        { key: 'inf', label: 'Inf', value: Number.POSITIVE_INFINITY },
        { key: 'ok', label: 'Ok', value: 2 },
      ];
      const plain = renderDom(BarChartHarness, { buckets });
      expect(columns(plain.container).map(barHeight)).toEqual([null, null, null, '100%']);
      expect(columns(plain.container).map((column) => column.querySelector('.value')?.textContent ?? null)).toEqual([null, null, null, '2 ч']);
      expect(within(plain.getByRole('table')).getAllByRole('cell').map((cell) => cell.textContent)).toEqual(['не играли', 'не играли', 'не играли', '2 ч']);
      plain.unmount();
      const chosen = renderDom(BarChartHarness, { buckets, selectable: true, start: 'inf', values: 'selected' });
      expect(within(chosen.container).getAllByRole('radio').map((radio) => radio.getAttribute('aria-label'))).toEqual(['NaN: не играли', 'Neg: не играли', 'Inf: не играли', 'Ok: 2 ч']);
      expect(columns(chosen.container).map((column) => column.querySelector('.value')?.textContent ?? null)).toEqual([null, null, null, null]);
    });
  });

  describe('the axis', () => {
    it('writes the ticks with the value format and the zero tick as a bare 0, not as the format words zero', () => {
      const { container } = renderDom(BarChartHarness, { buckets: DAYS, scale: { max: 8, step: 4 } });
      expect([...container.querySelectorAll('.axis .mark')].map((mark) => mark.textContent)).toEqual(['0', '4 ч', '8 ч']);
    });

    it('takes a tick format of its own, which writes the zero tick too', () => {
      const { container } = renderDom(BarChartHarness, { buckets: DAYS, scale: { max: 8, step: 4 }, tickFormat: (value: number) => `${value}h` });
      expect([...container.querySelectorAll('.axis .mark')].map((mark) => mark.textContent)).toEqual(['0h', '4h', '8h']);
    });
  });

  describe('motion', () => {
    const grows = (root: HTMLElement) => root.querySelector('.bar-chart')?.classList.contains('grow');

    it('lets the bars grow from zero with motion on', () => {
      const { container } = renderDom(BarChartHarness, { buckets: DAYS });
      expect(grows(container)).toBe(true);
    });

    it('draws them at once with animations off', () => {
      document.documentElement.classList.add('no-anim');
      const { container } = renderDom(BarChartHarness, { buckets: DAYS });
      expect(grows(container)).toBe(false);
    });
  });
});
