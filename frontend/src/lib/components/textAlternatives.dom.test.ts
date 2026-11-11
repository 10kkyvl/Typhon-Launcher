import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/playlog', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/playlog')>()),
  getStats: vi.fn(async (period: string) => ({
    period,
    totalSeconds: 5400,
    sessions: 2,
    longest: null,
    top: [],
    days: [
      { date: '2026-11-09', seconds: 0 },
      { date: '2026-11-10', seconds: 5400 },
    ],
  })),
}));

import { msg } from '../i18n';
import { weekSummary } from '../profile/week';
import type { ReleaseGroup } from '../services/sources';
import { description, hintOnFocus, tabbable } from '../testing/hints';
import { playtime, shortDate } from '../utils/format';
import WeekActivity from '../../routes/activity/WeekActivity.svelte';
import Stats from '../../routes/stats/Stats.svelte';
import ReleaseList from './ReleaseList.svelte';
import SourceBadge from './SourceBadge.svelte';

// What a native title said on a badge or a chart bar, which no keyboard reaches and screen readers
// skip, now reaches a user without a mouse.

/** The text a screen reader reads out of `element`: everything not hidden from it. */
function readText(element: Element): string {
  const parts: string[] = [];
  const walk = (node: Node) => {
    if (node instanceof Element && (node.getAttribute('aria-hidden') === 'true' || node.hasAttribute('hidden'))) return;
    if (node.nodeType === Node.TEXT_NODE) parts.push(node.textContent ?? '');
    node.childNodes.forEach(walk);
  };
  walk(element);
  return parts.join(' ').replace(/\s+/g, ' ').trim();
}

let app: HTMLDivElement;

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

describe('text alternatives for hints on what no keyboard reaches', () => {
  it('a source badge reads as the full sentence its short label stands for', () => {
    render(SourceBadge, { target: app, props: { source: 'steam' } });
    const badge = app.querySelector('.badge') as HTMLElement;
    expect(readText(badge)).toBe(msg('games.externalBadgeTitle', { provider: msg('games.sourceSteam') }));
  });

  it("the week chart is a list whose items read as each day's date and play time", () => {
    const today = new Date(2026, 10, 11);
    const week = weekSummary(
      [{ date: '2026-11-10', entries: [{ game: { id: 'g', title: 'Hades', cover: '', archived: false, playtimeSeconds: 3600, status: '' }, seconds: 3600 }] }],
      today,
    );
    render(WeekActivity, { target: app, props: { week } });
    const list = screen.getByRole('list', { name: msg('transfers.activityWeekTitle') });
    const items = within(list).getAllByRole('listitem');
    expect(items).toHaveLength(7);
    const monday = week.days.find((day) => day.date === '2026-11-10');
    expect(items.map(readText)).toContain(`${shortDate(monday?.at as Date)} · ${playtime(3600)}`);
    expect(items.map(readText)).toContain(`${shortDate(week.days[0].at)} · ${msg('transfers.activityWeekIdle')}`);
  });

  it("the stats chart is a list whose items read as each bar's date and play time", async () => {
    render(Stats, { target: app });
    const list = await screen.findByRole('list', { name: msg('stats.chartTitle') });
    const texts = within(list).getAllByRole('listitem').map(readText);
    expect(texts).toEqual([
      `${shortDate(new Date(2026, 10, 9))} · ${msg('stats.chartIdle')}`,
      `${shortDate(new Date(2026, 10, 10))} · ${playtime(5400)}`,
    ]);
  });

  it('the other sources of a release are named to a keyboard and a screen reader', async () => {
    const release = { id: 'r', sourceId: 's', rawTitle: 'Hades', title: 'Hades', normalizedTitle: 'hades', canonicalGameId: 'g', version: '1.0', size: 1024, uploadedAt: null, uris: [], matchStatus: 'matched', matchConfidence: 1, matchMethod: 'override', availability: 'available', firstSeenAt: '', lastSeenAt: '', createdAt: '' };
    const groups = [{ sourceName: 'Main feed', release, duplicates: [{ sourceName: 'Mirror' }, { sourceName: 'Archive' }] }] as unknown as ReleaseGroup[];
    render(ReleaseList, { target: app, props: { groups, ondownload: () => {} } });
    const more = app.querySelector('.release-dup') as HTMLElement;
    expect(tabbable(more)).toBe(true);
    expect(description(more)).toBe('Mirror, Archive');
    expect(await hintOnFocus(more)).toBe('Mirror, Archive');
  });
});
