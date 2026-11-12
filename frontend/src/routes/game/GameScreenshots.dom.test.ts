import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { listShots, screenshotsStatus, type Shot } from '../../lib/services/screenshots';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import GameScreenshots from './GameScreenshots.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/screenshots', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/screenshots')>()),
  listShots: vi.fn(),
  screenshotsStatus: vi.fn(),
}));

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(screenshotsStatus).mockResolvedValue({ enabled: true, hotkey: 'F12' } as Awaited<ReturnType<typeof screenshotsStatus>>);
});

afterEach(() => {
  vi.mocked(listShots).mockReset();
  vi.mocked(screenshotsStatus).mockReset();
  toasts.set([]);
  vi.restoreAllMocks();
});

describe('GameScreenshots: loading and nothing yet', () => {
  it('shows the base spinner named for what loads, then the empty state', async () => {
    let answer!: (shots: Shot[]) => void;
    vi.mocked(listShots).mockReturnValue(new Promise((resolve) => (answer = resolve)));
    renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });

    const status = screen.getByRole('status');
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('screenshots.loading'));

    answer([]);
    const empty = (await screen.findByText(msg('screenshots.empty', { hotkey: 'F12' }))).closest('.empty');
    expect(empty?.classList.contains('inline')).toBe(true);
    expect(screen.queryByRole('status')).toBeNull();
  });
});
