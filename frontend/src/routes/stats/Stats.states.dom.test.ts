import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/playlog', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/playlog')>()),
  getStats: vi.fn(),
  onPlaylogRecorded: vi.fn(() => () => {}),
}));

import { locale, msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { getStats, onPlaylogRecorded, type Stats as StatsData, type StatsPeriod } from '../../lib/services/playlog';
import { libraryGames } from '../../lib/stores/library';
import { renderDom } from '../../lib/testing/dom';
import { hintOnFocus } from '../../lib/testing/hints';
import { playtime, shortDate, weekday } from '../../lib/utils/format';
import Stats from './Stats.svelte';

const TO = new Date(2026, 10, 11);

const iso = (date: Date) =>
  `${date.getFullYear()}-${`${date.getMonth() + 1}`.padStart(2, '0')}-${`${date.getDate()}`.padStart(2, '0')}`;

const back = (days: number) => new Date(TO.getFullYear(), TO.getMonth(), TO.getDate() - days);

/** `count` zero-filled local days ending on TO, as Go sends them; `played` gives a day's seconds by its index. */
function stats(period: StatsPeriod, count: number, played: (index: number) => number, extra: Partial<StatsData> = {}): StatsData {
  const days = Array.from({ length: count }, (_, index) => ({ date: iso(back(count - 1 - index)), seconds: played(index) }));
  const totalSeconds = days.reduce((sum, day) => sum + day.seconds, 0);
  return {
    period,
    from: days[0].date,
    to: iso(TO),
    totalSeconds,
    sessions: totalSeconds > 0 ? 3 : 0,
    days,
    top: totalSeconds > 0 ? [{ gameId: 'g1', title: 'Hades', seconds: totalSeconds, sessions: 3 }] : [],
    longest: null,
    ...extra,
  };
}

const week = (extra: Partial<StatsData> = {}) => stats('week', 7, (index) => (index === 5 ? 5400 : 0), extra);
const month = () => stats('month', 30, (index) => (index % 4 === 0 ? 1800 : 0));
const year = () => stats('year', 365, (index) => (index % 30 === 0 ? 3600 : 0));

/**
 * The chart's words, bucket by bucket, as a user without sight or a pointer reaches them: each bar is a
 * radio of the chart's group, named by its date and its value.
 */
async function chartWords(name: string): Promise<string[]> {
  const group = await screen.findByRole('radiogroup', { name });
  return within(group).getAllByRole('radio').map((radio) => radio.getAttribute('aria-label') ?? '');
}

/** The bars of the chart named `name`. */
const bars = (name: string) => within(screen.getByRole('radiogroup', { name })).getAllByRole('radio');

/** The value labels drawn over the bars, by bar: null where none is drawn. */
const valueLabels = (container: HTMLElement) =>
  [...container.querySelectorAll('.chart-box .col')].map((col) => col.querySelector('.value')?.textContent?.trim() ?? null);

/** A period's control by its name, whatever form the switch takes. */
function periodControl(name: string): HTMLElement {
  return screen.queryByRole('radio', { name }) ?? screen.getByRole('button', { name });
}

const valueWords = (seconds: number) => (seconds > 0 ? playtime(seconds) : msg('stats.chartIdle'));
const barWords = (when: string, seconds: number) => msg('common.chartBucket', { label: when, value: valueWords(seconds) });

let recorded: (() => void) | undefined;

beforeEach(() => {
  vi.mocked(getStats).mockReset();
  vi.mocked(onPlaylogRecorded).mockImplementation((handler) => {
    recorded = handler;
    return () => {
      recorded = undefined;
    };
  });
});

afterEach(() => {
  libraryGames.set([]);
  recorded = undefined;
});

describe('Stats: the period switch', () => {
  it('asks for the chosen period and shows the spinner, not the old period, while it loads', async () => {
    let answer!: (value: StatsData) => void;
    vi.mocked(getStats)
      .mockResolvedValueOnce(week())
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { user, findByText } = renderDom(Stats);
    await findByText(msg('stats.totalLabel'));

    await user.click(periodControl(msg('stats.periodMonth')));

    expect(vi.mocked(getStats).mock.calls.map(([period]) => period)).toEqual(['week', 'month']);
    expect(screen.getByText(msg('stats.loading')).closest('.spinner[role="status"]')).not.toBeNull();
    expect(screen.queryByText(msg('stats.totalLabel'))).toBeNull();

    answer(month());
    expect(await chartWords(msg('stats.chartTitle'))).toHaveLength(30);
  });

  it('a press on the period already chosen asks for nothing', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week());
    const { user, findByText } = renderDom(Stats);
    await findByText(msg('stats.totalLabel'));

    await user.click(periodControl(msg('stats.periodWeek')));

    expect(getStats).toHaveBeenCalledTimes(1);
    expect(screen.getByText(msg('stats.totalLabel'))).toBeTruthy();
  });
});

describe('Stats: the period switch is one choice of three', () => {
  it('is a radio group named by what it chooses, the chosen period checked, one Tab stop', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week());
    const { findByText } = renderDom(Stats);
    await findByText(msg('stats.totalLabel'));

    const group = screen.getByRole('radiogroup', { name: msg('stats.periodLabel') });
    const radios = within(group).getAllByRole('radio');
    expect(radios.map((radio) => radio.textContent?.trim())).toEqual([msg('stats.periodWeek'), msg('stats.periodMonth'), msg('stats.periodYear')]);
    expect(radios.map((radio) => radio.getAttribute('aria-checked'))).toEqual(['true', 'false', 'false']);
    expect(radios.map((radio) => radio.tabIndex)).toEqual([0, -1, -1]);
  });

  it('an arrow key chooses the next period and asks for it', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockReturnValueOnce(new Promise(() => {}));
    const { findByText, user } = renderDom(Stats);
    await findByText(msg('stats.totalLabel'));
    screen.getByRole('radio', { name: msg('stats.periodWeek') }).focus();

    await user.keyboard('{ArrowRight}');

    const monthRadio = screen.getByRole('radio', { name: msg('stats.periodMonth') });
    expect(monthRadio.getAttribute('aria-checked')).toBe('true');
    expect(document.activeElement).toBe(monthRadio);
    expect(vi.mocked(getStats).mock.calls.map(([period]) => period)).toEqual(['week', 'month']);
  });
});

describe('Stats: a reload while the period is shown', () => {
  it('reloads the shown period when a session is recorded', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(month()).mockResolvedValueOnce(month());
    const { user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    await user.click(periodControl(msg('stats.periodMonth')));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitle'))).toHaveLength(30));

    recorded?.();

    expect(vi.mocked(getStats).mock.calls.map(([period]) => period)).toEqual(['week', 'month', 'month']);
  });

  it('keeps the shown figures, dimmed, until the new ones come', async () => {
    let answer!: (value: StatsData) => void;
    vi.mocked(getStats)
      .mockResolvedValueOnce(week())
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { container, findByText } = renderDom(Stats);
    await findByText(playtime(5400), { selector: '.value' });

    recorded?.();

    await vi.waitFor(() => expect(container.querySelector('.stale')).not.toBeNull());
    expect(screen.getByText(msg('stats.totalLabel'))).toBeTruthy();
    expect(screen.queryByText(msg('stats.loading'))).toBeNull();

    answer(week({ totalSeconds: 9000, days: week().days?.map((day, index) => ({ ...day, seconds: index === 6 ? 9000 : 0 })) }));
    await findByText(playtime(9000), { selector: '.value' });
    expect(container.querySelector('.stale')).toBeNull();
  });
});

describe('Stats: the longest session', () => {
  it('says its length, the game and the day', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
    vi.mocked(getStats).mockResolvedValueOnce(week({ longest: { gameId: 'g1', title: 'Hades', seconds: 4200, date: '2026-11-10' } }));
    const { findByText } = renderDom(Stats);

    const tile = (await findByText(msg('stats.longestLabel'))).parentElement as HTMLElement;
    expect(tile.textContent).toContain(playtime(4200));
    expect(tile.textContent).toContain(msg('stats.longestSub', { game: 'Hades', date: shortDate(new Date(2026, 10, 10)) }));
  });

  it('names a game gone from the library by the title the log kept', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week({ longest: { gameId: 'gone', title: '', seconds: 4200, date: '2026-11-10' } }));
    const { findByText } = renderDom(Stats);

    const tile = (await findByText(msg('stats.longestLabel'))).parentElement as HTMLElement;
    expect(tile.textContent).toContain(msg('stats.deletedGame'));
  });

  it('says there is none when the period has no session to name', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week({ longest: null }));
    const { findByText } = renderDom(Stats);

    const tile = (await findByText(msg('stats.longestLabel'))).parentElement as HTMLElement;
    expect(tile.textContent).toContain(msg('stats.longestNone'));
  });
});

describe('Stats: the chart in words', () => {
  it('a week says each of its seven days and the time played on it', async () => {
    const data = week();
    vi.mocked(getStats).mockResolvedValueOnce(data);
    renderDom(Stats);

    const expected = (data.days ?? []).map((day, index) => barWords(shortDate(back(6 - index)), day.seconds));
    expect(await chartWords(msg('stats.chartTitle'))).toEqual(expected);
    expect(expected[5]).toContain(playtime(5400));
    expect(expected[0]).toContain(msg('stats.chartIdle'));
  });

  it('a month says each of its thirty days', async () => {
    const data = month();
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(data);
    const { user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    await user.click(periodControl(msg('stats.periodMonth')));

    const expected = (data.days ?? []).map((day, index) => barWords(shortDate(back(29 - index)), day.seconds));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitle'))).toEqual(expected));
  });

  it('a year says each month it spans with the time played in it', async () => {
    const data = year();
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(data);
    const { user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    await user.click(periodControl(msg('stats.periodYear')));

    const monthLong = new Intl.DateTimeFormat(get(locale), { month: 'long', year: 'numeric' });
    const totals = new Map<string, { at: Date; seconds: number }>();
    for (const day of data.days ?? []) {
      const key = day.date.slice(0, 7);
      const found = totals.get(key);
      if (found) found.seconds += day.seconds;
      else totals.set(key, { at: new Date(Number(key.slice(0, 4)), Number(key.slice(5, 7)) - 1, 1), seconds: day.seconds });
    }
    const expected = [...totals.values()].map(({ at, seconds }) => barWords(monthLong.format(at), seconds));
    expect(expected).toHaveLength(13);
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitleYear'))).toEqual(expected));
  });

  it('marks the week days with their weekday under the bars', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week());
    const { container } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));

    const ticks = [...container.querySelectorAll('.tick')].map((tick) => tick.textContent?.trim());
    expect(ticks).toEqual(Array.from({ length: 7 }, (_, index) => weekday(back(6 - index))));
  });
});

describe('Stats: the sections', () => {
  it('the chart and the most played are sections named by level-2 headings, the list with its count', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week({ top: [
      { gameId: 'g1', title: 'Hades', seconds: 3000, sessions: 2 },
      { gameId: 'g2', title: 'Celeste', seconds: 2400, sessions: 1 },
    ] }));
    renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));

    const headings = screen.getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent?.replace(/\s+/g, ' ').trim());
    expect(headings).toEqual([msg('stats.chartTitle'), `${msg('stats.topTitle')} 2`]);
    const chart = screen.getByRole('region', { name: msg('stats.chartTitle') });
    expect(within(chart).getByRole('radiogroup', { name: msg('stats.chartTitle') })).toBeTruthy();
    const top = screen.getByRole('region', { name: `${msg('stats.topTitle')} 2` });
    expect(within(top).getAllByRole('listitem')).toHaveLength(2);
  });
});

describe('Stats: the shared bar chart', () => {
  // Under the bars: the day of the month every fifth day, the other days blank.
  it('a month marks every fifth day under its bars', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(month());
    const { container, user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    await user.click(periodControl(msg('stats.periodMonth')));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitle'))).toHaveLength(30));

    const ticks = [...container.querySelectorAll('.tick')].map((tick) => tick.textContent?.trim());
    expect(ticks).toEqual(Array.from({ length: 30 }, (_, index) => (index % 5 === 0 ? `${back(29 - index).getDate()}` : '')));
  });

  it('sets today apart, and in a year the month today is in', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(year());
    const { container, user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    const current = () => [...container.querySelectorAll('.col')].map((col) => col.classList.contains('current'));

    expect(current()).toEqual([false, false, false, false, false, false, true]);

    await user.click(periodControl(msg('stats.periodYear')));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitleYear'))).toHaveLength(13));
    expect(current()).toEqual([...Array(12).fill(false), true]);
  });

  it('draws an hour axis whose zero is a bare number, not the words for no play', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week());
    const { container } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));

    const marks = [...container.querySelectorAll('.axis .mark')].map((mark) => mark.textContent?.trim());
    expect(marks[0]).toBe('0');
    expect(marks.at(-1)).toBe(playtime(2 * 3600));
    expect(marks).not.toContain(msg('stats.chartIdle'));
  });

  it('is one choice of its bars, nothing chosen at first, one Tab stop, with no table repeating them and no title', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week());
    const { container } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));

    const all = bars(msg('stats.chartTitle'));
    expect(all.map((bar) => bar.getAttribute('aria-checked'))).toEqual(Array(7).fill('false'));
    expect(all.filter((bar) => bar.tabIndex === 0)).toEqual([all[6]]);
    expect(screen.queryByRole('table')).toBeNull();
    expect(container.querySelector('[title]')).toBeNull();
  });
});

describe('Stats: a bar chosen on the chart', () => {
  it('a week and a year label every bar that has play; a month labels none until one is chosen', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(month()).mockResolvedValueOnce(year());
    const { container, user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    expect(valueLabels(container)).toEqual([null, null, null, null, null, playtime(5400), null]);

    await user.click(periodControl(msg('stats.periodMonth')));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitle'))).toHaveLength(30));
    expect(valueLabels(container)).toEqual(Array(30).fill(null));

    await user.click(periodControl(msg('stats.periodYear')));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitleYear'))).toHaveLength(13));
    expect(valueLabels(container).filter((label) => label !== null).length).toBeGreaterThan(1);
  });

  it('in a month, Tab reaches today, the arrows move the choice, and the chosen bar shows its value', async () => {
    const data = month();
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(data);
    const { container, user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    await user.click(periodControl(msg('stats.periodMonth')));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitle'))).toHaveLength(30));
    const all = bars(msg('stats.chartTitle'));
    const label = (seconds: number) => playtime(seconds);

    all[29].focus();
    await user.keyboard('{ArrowLeft}');
    expect(document.activeElement).toBe(all[28]);
    expect(all[28].getAttribute('aria-checked')).toBe('true');
    expect(valueLabels(container)[28]).toBe(label((data.days ?? [])[28].seconds));
    expect(valueLabels(container).filter((value) => value !== null)).toHaveLength(1);

    await user.keyboard('{Home}');
    expect(document.activeElement).toBe(all[0]);
    expect(valueLabels(container)[0]).toBe(label((data.days ?? [])[0].seconds));
    expect(valueLabels(container).filter((value) => value !== null)).toHaveLength(1);
    expect(all.filter((bar) => bar.tabIndex === 0)).toEqual([all[0]]);
  });

  it('a bar says its date and value in a hint on focus and on hover', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week());
    const { user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    const all = bars(msg('stats.chartTitle'));
    const hint = () => document.querySelector('[data-part="tooltip"]')?.textContent?.trim() ?? null;

    expect(await hintOnFocus(all[6])).toBe(barWords(shortDate(TO), 0));

    await user.hover(all[5]);
    await vi.waitFor(() => expect(hint()).toBe(barWords(shortDate(back(1)), 5400)));
  });

  it('a change of period takes the choice away', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(month()).mockResolvedValueOnce(week());
    const { user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    await user.click(bars(msg('stats.chartTitle'))[5]);
    expect(bars(msg('stats.chartTitle'))[5].getAttribute('aria-checked')).toBe('true');

    await user.click(periodControl(msg('stats.periodMonth')));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitle'))).toHaveLength(30));
    expect(bars(msg('stats.chartTitle')).filter((bar) => bar.getAttribute('aria-checked') === 'true')).toEqual([]);

    await user.click(periodControl(msg('stats.periodWeek')));
    await vi.waitFor(async () => expect(await chartWords(msg('stats.chartTitle'))).toHaveLength(7));
    expect(bars(msg('stats.chartTitle')).filter((bar) => bar.getAttribute('aria-checked') === 'true')).toEqual([]);
  });

  it('a recorded session keeps the bar that is chosen', async () => {
    vi.mocked(getStats).mockResolvedValueOnce(week()).mockResolvedValueOnce(week());
    const { user } = renderDom(Stats);
    await chartWords(msg('stats.chartTitle'));
    await user.click(bars(msg('stats.chartTitle'))[5]);

    recorded?.();

    await vi.waitFor(() => expect(getStats).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(screen.queryByText(msg('stats.loading'))).toBeNull());
    expect(bars(msg('stats.chartTitle'))[5].getAttribute('aria-checked')).toBe('true');
  });
});
