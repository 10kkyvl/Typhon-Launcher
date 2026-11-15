import { describe, expect, it, vi } from 'vitest';
import { locale } from '../i18n';
import { renderDom } from '../testing/dom';
import { hintOnFocus } from '../testing/hints';
import { bytesSize } from '../utils/format';
import DiskMeter from './DiskMeter.svelte';
import type { MeterSegment } from './diskMeter';

const GB = 1024 ** 3;
const SEGMENTS: MeterSegment[] = [
  { key: 'games', label: 'Игры', bytes: 120 * GB, tone: 'game' },
  { key: 'other', label: 'Другое', bytes: 30 * GB, tone: 'other' },
];

// Every attribute and text node of the meter: a NaN or an Infinity anywhere is a defect.
function everything(root: HTMLElement): string {
  const attrs = [root, ...root.querySelectorAll('*')].flatMap((el) => [...el.attributes].map((a) => a.value));
  return [root.textContent ?? '', ...attrs].join(' ');
}

const parts = (root: HTMLElement) => [...root.querySelectorAll<HTMLElement>('.part')];
// Each legend entry as one line of words, every run of spaces (no-break ones too) as one space.
const words = (text: string) => text.replace(/\s+/g, ' ').trim();
const legendLines = (list: HTMLElement) => [...list.querySelectorAll('li')].map((li) => words(li.textContent ?? ''));

describe('DiskMeter', () => {
  it('is one image whose text alternative carries the numbers of the legend', () => {
    const { getByRole, container } = renderDom(DiskMeter, { label: 'Диск C:', segments: SEGMENTS, total: 200 * GB, free: 50 * GB });
    const image = getByRole('img');
    const name = image.getAttribute('aria-label') ?? '';
    for (const text of ['Диск C:', `Игры: ${bytesSize(120 * GB)}`, `Другое: ${bytesSize(30 * GB)}`, `Свободно ${bytesSize(50 * GB)} из ${bytesSize(200 * GB)}`]) {
      expect(name).toContain(text);
    }
    const legend = getByRole('list');
    expect(legend.textContent).toContain('Игры');
    expect(legend.textContent).toContain(bytesSize(120 * GB));
    expect(legend.textContent).toContain(bytesSize(50 * GB));
    expect(parts(container).map((p) => p.dataset.tone)).toEqual(['game', 'other']);
  });

  // A screen reader reads the alternative as one run: its parts must stay apart in both languages.
  for (const [lang, want] of [
    ['ru', () => `Диск C: Игры: ${bytesSize(120 * GB)}; Другое: ${bytesSize(30 * GB)}; Свободно ${bytesSize(50 * GB)} из ${bytesSize(200 * GB)}`],
    ['en', () => `Disk C: Игры: ${bytesSize(120 * GB)}; Другое: ${bytesSize(30 * GB)}; ${bytesSize(50 * GB)} free of ${bytesSize(200 * GB)}`],
  ] as const) {
    it(`separates the parts of the text alternative in ${lang}`, () => {
      locale.set(lang);
      const { getByRole } = renderDom(DiskMeter, { label: lang === 'ru' ? 'Диск C' : 'Disk C', segments: SEGMENTS, total: 200 * GB, free: 50 * GB });
      expect(getByRole('img').getAttribute('aria-label')).toBe(want());
    });
  }

  for (const total of [0, undefined, -1, Number.NaN]) {
    it(`says the total ${String(total)} is unknown, draws nothing and writes no NaN`, () => {
      const { getByRole, container } = renderDom(DiskMeter, { label: 'Диск', segments: SEGMENTS, total });
      expect(getByRole('img').getAttribute('aria-label')).toContain('Общий объём неизвестен');
      expect(container.textContent).toContain('Общий объём неизвестен');
      expect(parts(container)).toEqual([]);
      expect(everything(container)).not.toMatch(/NaN|Infinity/);
    });
  }

  it('draws a segment far under one pixel as a part of its own', () => {
    const { container } = renderDom(DiskMeter, {
      label: 'Диск',
      segments: [
        { key: 'big', label: 'Большая', bytes: 2000 * GB, tone: 'game' },
        { key: 'tiny', label: 'Крошечная', bytes: 1, tone: 'game-2' },
      ],
      total: 2000 * GB + 1,
    });
    expect(parts(container).map((p) => p.dataset.tone)).toEqual(['game', 'game-2']);
  });

  it('says a lower bound and an unknown size in the legend and the alternative', () => {
    const { getByRole } = renderDom(DiskMeter, {
      label: 'Диск',
      segments: [
        { key: 'a', label: 'Загрузки', bytes: 4 * GB, tone: 'data', sizeUnknown: true },
        { key: 'b', label: 'Снимки', bytes: 0, tone: 'data', sizeUnknown: true },
      ],
      total: 100 * GB,
    });
    const said = `не меньше ${bytesSize(4 * GB)}`;
    expect(getByRole('img').getAttribute('aria-label')).toContain(`Загрузки: ${said}`);
    expect(getByRole('img').getAttribute('aria-label')).toContain('Снимки: размер неизвестен');
    expect(getByRole('list').textContent).toContain(said);
    expect(getByRole('list').textContent).toContain('размер неизвестен');
  });

  it('draws the parts of an overflow in proportion, each with its real size, and says the sizes do not add up', () => {
    const { container, getByRole } = renderDom(DiskMeter, {
      label: 'Диск D',
      segments: [
        { key: 'a', label: 'Игры', bytes: 800 * GB, tone: 'game' },
        { key: 'b', label: 'Другое', bytes: 400 * GB, tone: 'other' },
      ],
      total: 1000 * GB,
      free: 0,
    });
    const bases = parts(container).map((p) => parseFloat(p.style.flexBasis));
    expect(bases[0] / bases[1]).toBeCloseTo(2, 9);
    expect(bases[0] + bases[1]).toBeCloseTo(100, 9);
    expect(getByRole('img').getAttribute('aria-label')).toBe(
      `Диск D: Игры: ${bytesSize(800 * GB)}; Другое: ${bytesSize(400 * GB)}; Размеры в сумме больше общего объёма; Всего ${bytesSize(1000 * GB)}`,
    );
    const legend = getByRole('list');
    expect(legendLines(legend)).toEqual(
      [`Игры ${bytesSize(800 * GB)}`, `Другое ${bytesSize(400 * GB)}`, `Размеры в сумме больше общего объёма; Всего ${bytesSize(1000 * GB)}`].map(words),
    );
  });

  it('draws and lists the space no segment accounts for as other files, not as free', () => {
    const { container, getByRole } = renderDom(DiskMeter, {
      label: 'Диск E',
      segments: [{ key: 'games', label: 'Игры', bytes: 600 * GB, tone: 'game' }],
      total: 1000 * GB,
      free: 300 * GB,
    });
    expect(parts(container).map((p) => [p.dataset.tone, parseFloat(p.style.flexBasis)])).toEqual([
      ['game', 60],
      ['other', 10],
    ]);
    expect(getByRole('img').getAttribute('aria-label')).toBe(
      `Диск E: Игры: ${bytesSize(600 * GB)}; Другие файлы: ${bytesSize(100 * GB)}; Свободно ${bytesSize(300 * GB)} из ${bytesSize(1000 * GB)}`,
    );
    expect(legendLines(getByRole('list'))).toEqual(
      [`Игры ${bytesSize(600 * GB)}`, `Другие файлы ${bytesSize(100 * GB)}`, `Свободно ${bytesSize(300 * GB)}`, `Всего ${bytesSize(1000 * GB)}`].map(words),
    );
  });

  it('in the compact form, is the bar and one line', () => {
    const { container, queryByRole, getByRole } = renderDom(DiskMeter, { label: 'Папка', segments: SEGMENTS, total: 200 * GB, free: 50 * GB, compact: true });
    expect(queryByRole('list')).toBeNull();
    expect(getByRole('img')).toBeTruthy();
    expect(container.querySelector('.line')?.textContent).toBe(`Свободно ${bytesSize(50 * GB)} из ${bytesSize(200 * GB)}`);
  });

  describe('with segments to pick', () => {
    it('is a named group of buttons, one Tab stop, the arrows move between them', async () => {
      const onpick = vi.fn();
      const { getByRole, getAllByRole, user } = renderDom(DiskMeter, { label: 'Диск C:', segments: SEGMENTS, total: 200 * GB, onpick });
      expect(getByRole('group').getAttribute('aria-label')).toContain('Диск C:');
      const buttons = getAllByRole('button');
      expect(buttons.map((b) => b.getAttribute('aria-label'))).toEqual([`Игры: ${bytesSize(120 * GB)}`, `Другое: ${bytesSize(30 * GB)}`]);
      expect(buttons.map((b) => b.tabIndex)).toEqual([0, -1]);

      await user.tab();
      expect(document.activeElement).toBe(buttons[0]);
      await user.keyboard('{ArrowRight}');
      expect(document.activeElement).toBe(buttons[1]);
      expect(buttons.map((b) => b.tabIndex)).toEqual([-1, 0]);
      await user.keyboard('{ArrowRight}');
      expect(document.activeElement).toBe(buttons[1]);
      await user.keyboard('{Home}');
      expect(document.activeElement).toBe(buttons[0]);
      await user.keyboard('{End}');
      expect(document.activeElement).toBe(buttons[1]);
      await user.keyboard('{Enter}');
      expect(onpick).toHaveBeenCalledWith('other');
    });

    it('shows the name and size of a focused segment in a hint', async () => {
      const { getAllByRole } = renderDom(DiskMeter, { label: 'Диск', segments: SEGMENTS, total: 200 * GB, onpick: vi.fn() });
      const first = getAllByRole('button')[0];
      expect(await hintOnFocus(first)).toBe(first.getAttribute('aria-label'));
    });
  });
});
