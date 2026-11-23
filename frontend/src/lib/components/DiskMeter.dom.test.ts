import { within } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { locale } from '../i18n';
import FocusThiefHarness from '../testing/FocusThiefHarness.svelte';
import { renderDom } from '../testing/dom';
import { hintOnFocus } from '../testing/hints';
import { bytesSize } from '../utils/format';
import DiskMeter from './DiskMeter.svelte';
import { METER_WIDTH, gameTone, meter, type MeterPart, type MeterSegment, type MeterSize } from './diskMeter';

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

  // Free space is what the player acts on before installing: measured, it is said in an overflow too,
  // before the overflow sentence, and drawn as the track the parts leave.
  it('says a measured free space in an overflow, then the overflow, then the total, and draws it as track', () => {
    const { container, getByRole } = renderDom(DiskMeter, {
      label: 'Диск D',
      segments: [{ key: 'games', label: 'Игры', bytes: 600 * GB, tone: 'game' }],
      total: 1000 * GB,
      free: 500 * GB,
    });
    expect(parts(container).map((p) => parseFloat(p.style.flexBasis))).toEqual([(600 / 1100) * 100]);
    expect(getByRole('img').getAttribute('aria-label')).toBe(
      `Диск D: Игры: ${bytesSize(600 * GB)}; Свободно: ${bytesSize(500 * GB)}; Размеры в сумме больше общего объёма; Всего ${bytesSize(1000 * GB)}`,
    );
    expect(legendLines(getByRole('list'))).toEqual(
      [`Игры ${bytesSize(600 * GB)}`, `Свободно ${bytesSize(500 * GB)}`, `Размеры в сумме больше общего объёма; Всего ${bytesSize(1000 * GB)}`].map(words),
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

  it('says other files beside a lower bound as an upper bound, drawn and filled, in the legend and the alternative', () => {
    const { container, getByRole } = renderDom(DiskMeter, {
      label: 'Диск E',
      segments: [{ key: 'games', label: 'Игры', bytes: 600 * GB, tone: 'game', sizeUnknown: true }],
      total: 1000 * GB,
      free: 300 * GB,
    });
    expect(getByRole('img').getAttribute('aria-label')).toBe(
      `Диск E: Игры: не меньше ${bytesSize(600 * GB)}; Другие файлы: не больше ${bytesSize(100 * GB)}; Свободно ${bytesSize(300 * GB)} из ${bytesSize(1000 * GB)}`,
    );
    expect(legendLines(getByRole('list'))[1]).toBe(words(`Другие файлы не больше ${bytesSize(100 * GB)}`));
    expect(parts(container).map((p) => [p.dataset.tone, parseFloat(p.style.flexBasis)])).toEqual([
      ['game', 60],
      ['other', 10],
    ]);
    const swatches = [...container.querySelectorAll('.legend .swatch')];
    expect(swatches[1].classList.contains('hollow')).toBe(false);
  });

  // The bare track says "free". Where the free space was not measured, or cannot be derived beside a
  // size not fully measured, the space after the segments is outlined as other files and free space
  // together, and the free space is not said apart from it.
  describe('the space after the segments, not known to be free', () => {
    const REST = 'Другие файлы и свободное место';
    const restOf = (root: HTMLElement) => parts(root).filter((p) => p.classList.contains('rest'));

    it('is outlined and named as other files and free space when the free space was not measured', () => {
      const { container, getByRole } = renderDom(DiskMeter, { label: 'Диск F', segments: [{ key: 'games', label: 'Игры', bytes: 600 * GB, tone: 'game' }], total: 1000 * GB, free: null });
      expect(parts(container).map((p) => [p.dataset.tone ?? null, p.classList.contains('rest'), parseFloat(p.style.flexBasis)])).toEqual([
        ['game', false, 60],
        [null, true, 40],
      ]);
      expect(getByRole('img').getAttribute('aria-label')).toBe(`Диск F: Игры: ${bytesSize(600 * GB)}; ${REST}: ${bytesSize(400 * GB)}; Всего ${bytesSize(1000 * GB)}`);
      expect(legendLines(getByRole('list'))).toEqual([`Игры ${bytesSize(600 * GB)}`, `${REST} ${bytesSize(400 * GB)}`, `Всего ${bytesSize(1000 * GB)}`].map(words));
      const swatch = container.querySelectorAll('.legend .swatch')[1];
      expect([swatch.classList.contains('rest'), swatch.hasAttribute('data-tone')]).toEqual([true, false]);
    });

    it('says it as an upper bound beside a game still being measured, the free space left out', () => {
      const { container, getByRole } = renderDom(DiskMeter, {
        label: 'Диск F',
        segments: [
          { key: 'games', label: 'Игры', bytes: 600 * GB, tone: 'game' },
          { key: 'new', label: 'Новая', bytes: 20 * GB, tone: 'game-2', sizeUnknown: true },
        ],
        total: 1000 * GB,
        compact: true,
      });
      expect(restOf(container).map((p) => parseFloat(p.style.flexBasis))).toEqual([38]);
      expect(container.querySelector('.line')?.textContent).toBe(`${REST}: не больше ${bytesSize(380 * GB)}; Всего ${bytesSize(1000 * GB)}`);
      expect(getByRole('img').getAttribute('aria-label')).toBe(
        `Диск F: Игры: ${bytesSize(600 * GB)}; Новая: не меньше ${bytesSize(20 * GB)}; ${REST}: не больше ${bytesSize(380 * GB)}; Всего ${bytesSize(1000 * GB)}`,
      );
    });

    it('is not drawn where the free space is derived from exact sizes: that is the track, said as free', () => {
      const { container } = renderDom(DiskMeter, { label: 'Диск F', segments: [{ key: 'games', label: 'Игры', bytes: 600 * GB, tone: 'game' }], total: 1000 * GB, compact: true });
      expect(restOf(container)).toEqual([]);
      expect(container.querySelector('.line')?.textContent).toBe(`Свободно ${bytesSize(400 * GB)} из ${bytesSize(1000 * GB)}`);
    });
  });

  // The words, the legend and the bar of every combination come from one model: each is checked
  // against the model `meter` returns for the same input (whose rules diskMeter.test.ts pins over the
  // same combinations), in the words the rules give. The drawing is also read against the words
  // alone, without the model: a model that lacks a part would draw and say the same wrong thing.
  describe('says, lists and draws one model', () => {
    const T = 1000 * GB;
    const REST = 'Другие файлы и свободное место';
    const escaped = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    // Read from the drawing and the words only: the bare track is said as free space of its width,
    // and a part drawn as the rest (no fill, an outline) is said as other files and free space of
    // its width, in the text alternative and in the words on screen.
    function drawingMatchesWords(container: HTMLElement, compact: boolean, overflow: boolean) {
      const drawn = parts(container);
      const bare = 100 - drawn.reduce((sum, p) => sum + parseFloat(p.style.flexBasis), 0);
      const alt = words(container.querySelector('.bar')?.getAttribute('aria-label') ?? '');
      const seen = compact ? words(container.querySelector('.line')?.textContent ?? '') : legendLines(container.querySelector('.legend') as HTMLElement).join(' | ');
      for (const [where, said] of [
        ['the text alternative', alt],
        ['the words on screen', seen],
      ]) {
        if (bare > 1e-6) {
          const free = overflow ? '\\S' : escaped(words(bytesSize((bare / 100) * T)));
          expect(said, `${where} says the bare track (${bare}%) as free space`).toMatch(new RegExp(`Свободно:? ${free}`));
        }
        for (const rest of drawn.filter((p) => p.classList.contains('rest'))) {
          const size = escaped(words(bytesSize((parseFloat(rest.style.flexBasis) / 100) * T)));
          expect(said, `${where} names the outlined part`).toMatch(new RegExp(`${REST}:? (не больше )?${size}`));
        }
        if (drawn.some((p) => p.classList.contains('rest'))) expect(said, `${where} does not say free space apart from the rest`).not.toContain('Свободно');
      }
      for (const rest of drawn.filter((p) => p.classList.contains('rest'))) expect(rest.dataset.tone, 'the rest is not a fill').toBeUndefined();
    }
    const plural = new Intl.PluralRules('ru');
    const MORE = { one: 'часть', few: 'части', many: 'частей', other: 'части' } as Record<string, string>;
    const size = (s: MeterSize) =>
      s.kind === 'unknown' ? 'размер неизвестен' : s.kind === 'atLeast' ? `не меньше ${bytesSize(s.bytes)}` : s.kind === 'atMost' ? `не больше ${bytesSize(s.bytes)}` : bytesSize(s.bytes);
    const name = (p: MeterPart) => (p.kind === 'other' ? 'Другие файлы' : p.kind === 'rest' ? REST : p.kind === 'more' ? `Ещё ${p.folded?.length} ${MORE[plural.select(p.folded?.length ?? 0)]}` : p.label);
    const seg = (key: string, bytes: number, sizeUnknown = false): MeterSegment => ({ key, label: `Игра ${key}`, bytes, tone: 'game', sizeUnknown });

    for (const overflow of [false, true])
      for (const free of ['measured', 'derived', 'null'] as const)
        for (const sizes of ['exact', 'lowerBound', 'unknown'] as const)
          for (const fold of [false, true])
            for (const compact of [false, true]) {
              it(`overflow ${overflow}, free ${free}, sizes ${sizes}, fold ${fold}, compact ${compact}`, () => {
                const segments = [seg('g1', 300 * GB), seg('g2', 200 * GB, sizes === 'lowerBound')];
                if (overflow) segments.push(seg('g3', 700 * GB));
                if (sizes === 'unknown') segments.push(seg('new', 0, true));
                if (fold) for (let i = 0; i < 60; i += 1) segments.push(seg(`t${i}`, GB / 100));
                const given = free === 'measured' ? (overflow ? 250 * GB : 300 * GB) : free === 'null' ? null : undefined;
                const m = meter(segments, T, given, compact ? METER_WIDTH.compact : METER_WIDTH.full);
                const { container, getByRole, queryByRole } = renderDom(DiskMeter, { label: 'Диск', segments, total: T, free: given, compact });

                expect(parts(container).map((p) => parseFloat(p.style.flexBasis))).toEqual(m.drawn.map((p) => p.share * 100));
                drawingMatchesWords(container, compact, overflow);
                const tail = m.overflow
                  ? [...(m.free ? [`Свободно: ${size(m.free)}`] : []), 'Размеры в сумме больше общего объёма', `Всего ${bytesSize(T)}`]
                  : m.free
                    ? [`Свободно ${size(m.free)} из ${bytesSize(T)}`]
                    : [`Всего ${bytesSize(T)}`];
                expect(getByRole('img').getAttribute('aria-label')).toBe(`Диск: ${[...m.parts.map((p) => `${name(p)}: ${size(p.size)}`), ...tail].join('; ')}`);
                if (compact) {
                  expect(queryByRole('list')).toBeNull();
                  const rest = m.parts.filter((p) => p.kind === 'rest').map((p) => `${REST}: ${size(p.size)}`);
                  expect(container.querySelector('.line')?.textContent).toBe([...rest, ...tail].join('; '));
                  return;
                }
                const ends = m.overflow ? [`Размеры в сумме больше общего объёма; Всего ${bytesSize(T)}`] : [`Всего ${bytesSize(T)}`];
                expect(legendLines(getByRole('list'))).toEqual(
                  [...m.parts.map((p) => `${name(p)} ${size(p.size)}`), ...(m.free ? [`Свободно ${size(m.free)}`] : []), ...ends].map(words),
                );
              });
            }
  });

  // A caller's own `other` segment is space it measured; what nothing accounts for is added beside it
  // in the same tone. The two are two entries under their own names, never merged into one.
  it("lists a caller's other segment and the space nothing accounts for as two entries", () => {
    const { container, getByRole } = renderDom(DiskMeter, {
      label: 'Диск',
      segments: [
        { key: 'games', label: 'Игры', bytes: 600 * GB, tone: 'game' },
        { key: 'system', label: 'Система', bytes: 50 * GB, tone: 'other' },
      ],
      total: 1000 * GB,
      free: 300 * GB,
    });
    expect(legendLines(getByRole('list')).slice(0, 3)).toEqual([`Игры ${bytesSize(600 * GB)}`, `Система ${bytesSize(50 * GB)}`, `Другие файлы ${bytesSize(50 * GB)}`].map(words));
    expect(getByRole('img').getAttribute('aria-label')).toBe(
      `Диск: Игры: ${bytesSize(600 * GB)}; Система: ${bytesSize(50 * GB)}; Другие файлы: ${bytesSize(50 * GB)}; Свободно ${bytesSize(300 * GB)} из ${bytesSize(1000 * GB)}`,
    );
    expect(parts(container).map((p) => [p.dataset.tone, parseFloat(p.style.flexBasis)])).toEqual([
      ['game', 60],
      ['other', 5],
      ['other', 5],
    ]);
  });

  // Bad input is said, not thrown: two segments under one key are both drawn, listed and pickable.
  it('draws and lists two segments that share a key, and reports that key for either', async () => {
    const onpick = vi.fn();
    const twice: MeterSegment[] = [
      { key: 'dup', label: 'Первая', bytes: 10 * GB, tone: 'game' },
      { key: 'dup', label: 'Вторая', bytes: 20 * GB, tone: 'game-2' },
    ];
    const { getAllByRole, getByRole, user } = renderDom(DiskMeter, { label: 'Диск', segments: twice, total: 100 * GB, onpick });
    expect(legendLines(getByRole('list')).slice(0, 2)).toEqual([`Первая ${bytesSize(10 * GB)}`, `Вторая ${bytesSize(20 * GB)}`].map(words));
    const buttons = getAllByRole('button');
    expect(buttons.map((b) => b.getAttribute('aria-label'))).toEqual([`Первая: ${bytesSize(10 * GB)}`, `Вторая: ${bytesSize(20 * GB)}`]);
    await user.click(buttons[1]);
    expect(onpick).toHaveBeenCalledWith('dup');
  });

  describe('a segment the bar cannot draw', () => {
    const SOME: MeterSegment[] = [
      { key: 'big', label: 'Большая', bytes: 50 * GB, tone: 'game' },
      { key: 'empty', label: 'Пустая', bytes: 0, tone: 'game-2' },
      { key: 'unmeasured', label: 'Не измерена', bytes: 0, tone: 'game-3', sizeUnknown: true },
    ];

    it('is listed, and picked from the legend when the meter picks', async () => {
      const onpick = vi.fn();
      const { getByRole, user } = renderDom(DiskMeter, { label: 'Диск', segments: SOME, total: 100 * GB, onpick });
      const legend = getByRole('list');
      expect(within(legend).getAllByRole('button').map((b) => b.textContent?.trim())).toEqual(['Пустая', 'Не измерена']);
      await user.click(within(legend).getByRole('button', { name: 'Не измерена' }));
      expect(onpick).toHaveBeenCalledWith('unmeasured');
      await user.click(within(legend).getByRole('button', { name: 'Пустая' }));
      expect(onpick).toHaveBeenLastCalledWith('empty');
    });

    it('is plain text in the legend of a meter that does not pick', () => {
      const { getByRole } = renderDom(DiskMeter, { label: 'Диск', segments: SOME, total: 100 * GB });
      expect(within(getByRole('list')).queryAllByRole('button')).toEqual([]);
      expect(legendLines(getByRole('list')).slice(1, 3)).toEqual(['Пустая', 'Не измерена'].map((name, i) => words(`${name} ${i === 0 ? bytesSize(0) : 'размер неизвестен'}`)));
    });
  });

  describe('with more parts than the bar can draw truthfully', () => {
    const games = (count: number): MeterSegment[] => Array.from({ length: count }, (_, i) => ({ key: `g${i}`, label: `Игра ${i}`, bytes: 10 * GB, tone: gameTone(i) }));

    it('folds the rest into one part, said with its count and size, and not a button unless asked', () => {
      const { container, getByRole, getAllByRole } = renderDom(DiskMeter, { label: 'Папка', segments: games(60), total: 600 * GB, compact: true, onpick: vi.fn() });
      expect(parts(container)).toHaveLength(19);
      expect(getAllByRole('button')).toHaveLength(18);
      expect(parts(container).at(-1)?.dataset.tone).toBe('more');
      expect(getByRole('group').getAttribute('aria-label')).toContain(`; Ещё 42 части: ${bytesSize(420 * GB)}; `);
    });

    it('lists the fold in the legend', () => {
      const { getByRole } = renderDom(DiskMeter, { label: 'Диск', segments: games(100), total: 1000 * GB });
      const lines = legendLines(getByRole('list'));
      expect(lines).toHaveLength(58 + 1 + 2);
      expect(lines[58]).toBe(words(`Ещё 42 части ${bytesSize(420 * GB)}`));
    });

    it("words the fold with the caller's own label", () => {
      const foldLabel = (count: number) => `ещё ${count} игры`;
      const { getByRole } = renderDom(DiskMeter, { label: 'Диск', segments: games(100), total: 1000 * GB, foldLabel });
      expect(legendLines(getByRole('list'))[58]).toBe(words(`ещё 42 игры ${bytesSize(420 * GB)}`));
      expect(getByRole('img').getAttribute('aria-label')).toContain(`; ещё 42 игры: ${bytesSize(420 * GB)}; `);
    });

    it('names the largest game first when every game is too small to draw, the rest in the fold', () => {
      const tiny: MeterSegment[] = [
        { key: 'celeste', label: 'Celeste', bytes: 1 * GB, tone: 'game' },
        { key: 'hades', label: 'Hades', bytes: 5 * GB, tone: 'game-2' },
        { key: 'inside', label: 'Inside', bytes: 3 * GB, tone: 'game-3' },
      ];
      const { getByRole } = renderDom(DiskMeter, { label: 'D', segments: tiny, total: 2048 * GB, free: 1948 * GB });
      expect(getByRole('img').getAttribute('aria-label')?.startsWith(`D: Hades: ${bytesSize(5 * GB)}; Ещё 2 части: ${bytesSize(4 * GB)}; `)).toBe(true);
    });

    // The full form's legend reaches every folded game when the meter picks: its fold entry opens to
    // list them by name and size, each a pick, in the caller's order.
    it('opens the fold in the legend of a meter that picks, and picks a folded segment from it', async () => {
      const onpick = vi.fn();
      const { container, user } = renderDom(DiskMeter, { label: 'Диск', segments: games(100), total: 1000 * GB, onpick });
      const legend = container.querySelector('.legend') as HTMLElement;
      const fold = within(legend).getByRole('button', { name: 'Ещё 42 части' });
      expect(fold.getAttribute('aria-expanded')).toBe('false');
      expect(within(legend).queryByRole('button', { name: 'Игра 58' })).toBeNull();
      await user.click(fold);
      expect(fold.getAttribute('aria-expanded')).toBe('true');
      const members = document.getElementById(fold.getAttribute('aria-controls') ?? '') as HTMLElement;
      expect(members.tagName).toBe('UL');
      expect(legend.contains(members)).toBe(true);
      expect(within(members).getAllByRole('button').map((b) => b.textContent?.trim())).toEqual(games(100).slice(58).map((g) => g.label));
      expect(legendLines(members)[0]).toBe(words(`Игра 58 ${bytesSize(10 * GB)}`));
      await user.click(within(members).getByRole('button', { name: 'Игра 70' }));
      expect(onpick).toHaveBeenCalledWith('g70');
      await user.click(fold);
      expect(fold.getAttribute('aria-expanded')).toBe('false');
      expect(document.getElementById(fold.getAttribute('aria-controls') ?? '')).toBeNull();
    });

    it('closes the fold when it leaves, so it does not come back open', async () => {
      const { container, rerender, user } = renderDom(DiskMeter, { label: 'Диск', segments: games(100), total: 1000 * GB, onpick: vi.fn() });
      const legend = () => container.querySelector('.legend') as HTMLElement;
      await user.click(within(legend()).getByRole('button', { name: 'Ещё 42 части' }));
      expect(container.querySelector('.members')).not.toBeNull();
      await rerender({ segments: games(99) });
      expect(within(legend()).getByRole('button', { name: 'Ещё 41 часть' }).getAttribute('aria-expanded')).toBe('true');
      await rerender({ segments: games(3) });
      expect(within(legend()).queryByRole('button', { name: /^Ещё/ })).toBeNull();
      await rerender({ segments: games(100) });
      expect(within(legend()).getByRole('button', { name: 'Ещё 42 части' }).getAttribute('aria-expanded')).toBe('false');
      expect(container.querySelector('.members')).toBeNull();
    });

    it("words the fold from the catalogue when the caller's label is empty", () => {
      const { getByRole } = renderDom(DiskMeter, { label: 'Диск', segments: games(100), total: 1000 * GB, foldLabel: () => '' });
      expect(legendLines(getByRole('list'))[58]).toBe(words(`Ещё 42 части ${bytesSize(420 * GB)}`));
      expect(getByRole('img').getAttribute('aria-label')).toContain(`; Ещё 42 части: ${bytesSize(420 * GB)}; `);
      const picking = renderDom(DiskMeter, { label: 'Диск', segments: games(60), total: 600 * GB, compact: true, onpick: vi.fn(), onpickfolded: vi.fn(), foldLabel: () => '' });
      expect(picking.getAllByRole('button').at(-1)?.getAttribute('aria-label')).toBe(`Ещё 42 части: ${bytesSize(420 * GB)}`);
    });

    it('keeps the fold plain text with its count and sum in the legend of a meter that does not pick', () => {
      const { getByRole } = renderDom(DiskMeter, { label: 'Диск', segments: games(100), total: 1000 * GB });
      expect(within(getByRole('list')).queryAllByRole('button')).toEqual([]);
      expect(legendLines(getByRole('list'))[58]).toBe(words(`Ещё 42 части ${bytesSize(420 * GB)}`));
    });

    it('makes the fold a button when the caller asks, which reports the keys folded in it', async () => {
      const onpickfolded = vi.fn();
      const { getAllByRole, user } = renderDom(DiskMeter, { label: 'Папка', segments: games(60), total: 600 * GB, compact: true, onpick: vi.fn(), onpickfolded });
      const buttons = getAllByRole('button');
      expect(buttons).toHaveLength(19);
      expect(buttons.at(-1)?.getAttribute('aria-label')).toBe(`Ещё 42 части: ${bytesSize(420 * GB)}`);
      await user.click(buttons.at(-1) as HTMLElement);
      expect(onpickfolded).toHaveBeenCalledWith(games(60).slice(18).map((g) => g.key));
    });
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

    // The Tab stop and a hand-off follow the segment, not its place in the row: a size that changes
    // can add, drop or move parts under the user's focus.
    describe('as the segments change', () => {
      const part = (key: string, gb: number): MeterSegment => ({ key, label: key, bytes: gb * GB, tone: 'game' });
      const ABC = [part('a', 30), part('b', 20), part('c', 10)];
      const named = (key: string) => `${key}: ${bytesSize(ABC.find((s) => s.key === key)?.bytes ?? 0)}`;
      const meterOf = (segments: MeterSegment[]) => renderDom(DiskMeter, { label: 'Диск', segments, total: 100 * GB, onpick: vi.fn() });

      it('keeps the Tab stop on the same segment when one is added before it', async () => {
        const { getAllByRole, rerender, user } = meterOf(ABC);
        await user.tab();
        await user.keyboard('{ArrowRight}');
        await user.tab();
        expect(document.activeElement).toBe(document.body);
        await rerender({ segments: [part('new', 5), ...ABC] });
        const stops = getAllByRole('button').filter((b) => b.tabIndex === 0);
        expect(stops.map((b) => b.getAttribute('aria-label'))).toEqual([named('b')]);
      });

      it('hands focus to the segment now in the place of a focused one that leaves', async () => {
        const { rerender, user } = meterOf(ABC);
        await user.tab();
        await user.keyboard('{ArrowRight}');
        expect(document.activeElement?.getAttribute('aria-label')).toBe(named('b'));
        await rerender({ segments: [ABC[0], ABC[2]] });
        expect(document.activeElement?.getAttribute('aria-label')).toBe(named('c'));
        expect((document.activeElement as HTMLElement).tabIndex).toBe(0);
      });

      it('hands focus to the neighbour when the last one leaves', async () => {
        const { rerender, user } = meterOf(ABC);
        await user.tab();
        await user.keyboard('{End}');
        await rerender({ segments: ABC.slice(0, 2) });
        expect(document.activeElement?.getAttribute('aria-label')).toBe(named('b'));
      });

      it('hands focus to the group when no segment is left to draw', async () => {
        const { rerender, user, getByRole } = meterOf(ABC.slice(0, 1));
        await user.tab();
        await rerender({ segments: [], total: 0 });
        expect(document.activeElement).toBe(getByRole('group'));
        expect(getByRole('group').dataset.state).toBe('unknown');
      });

      it('leaves focus on a part that stays while another is added or removed before it', async () => {
        const { rerender, user } = meterOf(ABC);
        await user.tab();
        await user.keyboard('{ArrowRight}');
        const b = document.activeElement;
        expect(b?.getAttribute('aria-label')).toBe(named('b'));
        await rerender({ segments: [part('new', 5), ...ABC] });
        expect(document.activeElement).toBe(b);
        await rerender({ segments: ABC.slice(1) });
        expect(document.activeElement).toBe(b);
      });

      // Another part of the page (a dialog opening) moves focus in its own effect in the same update:
      // the meter's hand-off runs after it and must not take focus back, whether the focused part
      // stays or leaves.
      for (const [name, next] of [
        ['stays', [ABC[1], ABC[2]]],
        ['leaves', [ABC[0], ABC[2]]],
      ] as const) {
        it(`does not take focus back from what moved it in the same update, when the focused part ${name}`, async () => {
          const { container, getByRole, rerender } = renderDom(FocusThiefHarness, { segments: ABC, steal: 0 });
          const b = within(container).getByRole('button', { name: named('b') });
          b.focus();
          await rerender({ segments: [...next], steal: 1 });
          expect(document.activeElement).toBe(getByRole('button', { name: 'Диалог' }));
        });
      }

      it('leaves focus alone when it was not on a segment', async () => {
        const { rerender, user } = meterOf(ABC);
        const outside = document.createElement('button');
        document.body.append(outside);
        await user.tab();
        outside.focus();
        await rerender({ segments: ABC.slice(1) });
        expect(document.activeElement).toBe(outside);
        outside.remove();
      });
    });

    // Every pick the meter owns hands focus by one rule when it leaves: to the pick of the same segment
    // wherever it now is (the bar or the legend), else to its neighbour among the picks it stood with,
    // else to the bar's Tab stop, else to the group.
    describe('as a focused pick in the legend leaves', () => {
      const part = (key: string, gb: number, sizeUnknown = false): MeterSegment => ({ key, label: key, bytes: gb * GB, tone: 'game', sizeUnknown });
      const meterOf = (segments: MeterSegment[]) => renderDom(DiskMeter, { label: 'Диск', segments, total: 100 * GB, onpick: vi.fn() });
      const legend = (root: HTMLElement) => root.querySelector('.legend') as HTMLElement;

      it('follows a segment from its name in the legend to its part on the bar when it is measured', async () => {
        const { container, rerender } = meterOf([part('a', 30), part('x', 0, true), part('y', 0, true)]);
        within(legend(container)).getByRole('button', { name: 'x' }).focus();
        await rerender({ segments: [part('a', 30), part('x', 10), part('y', 0, true)] });
        expect(document.activeElement?.getAttribute('aria-label')).toBe(`x: ${bytesSize(10 * GB)}`);
        expect((document.activeElement as HTMLElement).tabIndex).toBe(0);
      });

      it('follows a segment from the bar to its name in the legend when its size is no longer known', async () => {
        const { container, rerender, user } = meterOf([part('a', 30), part('b', 20), part('c', 10)]);
        await user.tab();
        await user.keyboard('{ArrowRight}');
        await rerender({ segments: [part('a', 30), part('b', 0, true), part('c', 10)] });
        expect(document.activeElement).toBe(within(legend(container)).getByRole('button', { name: 'b' }));
      });

      it('hands focus to the name now in the place of one that leaves, else to its neighbour', async () => {
        const { container, rerender } = meterOf([part('a', 30), part('x', 0, true), part('y', 0, true)]);
        within(legend(container)).getByRole('button', { name: 'x' }).focus();
        await rerender({ segments: [part('a', 30), part('y', 0, true)] });
        expect(document.activeElement).toBe(within(legend(container)).getByRole('button', { name: 'y' }));
        await rerender({ segments: [part('a', 30)] });
        expect(document.activeElement?.getAttribute('aria-label')).toBe(`a: ${bytesSize(30 * GB)}`);
      });

      it('hands focus to the group when no pick is left', async () => {
        const { container, rerender, getByRole } = meterOf([part('x', 0, true)]);
        within(legend(container)).getByRole('button', { name: 'x' }).focus();
        await rerender({ segments: [] });
        expect(document.activeElement).toBe(getByRole('group'));
      });

      describe('in the fold', () => {
        const games = (count: number, first = 10): MeterSegment[] => Array.from({ length: count }, (_, i) => part(`g${i}`, i === 0 ? first : 0.01));

        it('follows a folded segment to its part on the bar when it grows', async () => {
          const { container, rerender, user } = meterOf(games(5));
          await user.click(within(legend(container)).getByRole('button', { name: /^Ещё 4/ }));
          within(legend(container)).getByRole('button', { name: 'g2' }).focus();
          const grown = games(5).map((g) => (g.key === 'g2' ? part('g2', 20) : g));
          await rerender({ segments: grown });
          expect(document.activeElement?.getAttribute('aria-label')).toBe(`g2: ${bytesSize(20 * GB)}`);
        });

        it('hands focus on from the fold when it is gone', async () => {
          const { container, rerender, user } = meterOf(games(5));
          const fold = within(legend(container)).getByRole('button', { name: /^Ещё 4/ });
          await user.click(fold);
          expect(document.activeElement).toBe(fold);
          await rerender({ segments: games(1) });
          expect(document.activeElement?.getAttribute('aria-label')).toBe(`g0: ${bytesSize(10 * GB)}`);
        });
      });
    });
  });
});
