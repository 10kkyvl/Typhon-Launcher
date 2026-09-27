import { expect, it, vi } from 'vitest';
vi.mock('../../lib/stores/metadata', async () => { const { writable } = await import('svelte/store'); return { gameArt: writable({}) }; });
vi.mock('../../lib/stores/router', () => ({ navigate: vi.fn() }));
import { render } from 'svelte/server';
import WeekActivity from './WeekActivity.svelte';
import ProfileActivity from '../profile/ProfileActivity.svelte';
import { weekSummary } from '../../lib/profile/week';
import type { ActivityDay } from '../../lib/services/profile';

it('renders archived sessions in the week and profile without a broken library link', () => {
  const activity: ActivityDay[] = [{ date: '2026-09-10', entries: [{
    game: { id: 'removed', title: 'Archived game', cover: '', archived: true, playtimeSeconds: 7200, status: '' },
    seconds: 7200,
  }] }];
  const week = weekSummary(activity, new Date(2026, 8, 10));
  expect(week.totalSeconds).toBe(7200);
  const weekly = render(WeekActivity, { props: { week } }).body;
  const profile = render(ProfileActivity, { props: { days: activity, hidden: false } }).body;
  for (const html of [weekly, profile]) {
    expect(html).toContain('Archived game');
    expect(html).toMatch(/<button[^>]*disabled/);
  }
});
