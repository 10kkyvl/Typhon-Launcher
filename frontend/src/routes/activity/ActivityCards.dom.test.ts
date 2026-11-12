import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { weekSummary } from '../../lib/profile/week';
import { renderDom } from '../../lib/testing/dom';
import PopularGames from './PopularGames.svelte';
import WeekActivity from './WeekActivity.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));

describe('Activity cards with nothing to show', () => {
  it('say so with the base empty state', () => {
    renderDom(PopularGames, { games: [] });
    renderDom(WeekActivity, { week: weekSummary([], new Date(2026, 10, 11)) });
    expect(screen.getByText(msg('transfers.activityPopularEmpty')).closest('.empty')).toBeTruthy();
    expect(screen.getByText(msg('transfers.activityWeekEmpty')).closest('.empty')).toBeTruthy();
  });
});
