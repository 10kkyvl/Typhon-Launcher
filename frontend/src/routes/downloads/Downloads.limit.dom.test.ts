import { fireEvent, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { saveSettingsPatch, type Settings } from '../../lib/services/settings';
import { resetHistory, route } from '../../lib/stores/router';
import { settings } from '../../lib/stores/settings';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { rateLimitLabel } from '../../lib/utils/format';
import { SPEED_LIMIT_NO_LIMIT, speedLimitPosition } from '../../lib/utils/speedLimit';
import Downloads from './Downloads.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));

const MB = 1024 ** 2;

function stored(limit: number): Settings {
  const known: Partial<Settings> = { language: 'system', uiScale: 1, animationsEnabled: true, maxActiveDownloads: 2, downloadRateLimit: limit, downloadSchedule: [] };
  return known as Settings;
}

const buttonName = (value: string) => msg('downloads.page.limitButton', { value });
const limitButton = () =>
  within(document.querySelector('header.page-header') as HTMLElement).getByRole('button', { name: /^Лимит: |^Limit: / });
const slider = () => screen.getByRole('slider', { name: msg('downloads.page.limitLabel') });

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  settings.set(null);
  toasts.set([]);
  resetHistory();
  vi.mocked(saveSettingsPatch).mockReset();
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

async function openLimit(limit: number) {
  settings.set(stored(limit));
  const rendered = renderDom(Downloads);
  limitButton().focus();
  await rendered.user.keyboard('{Enter}');
  return rendered;
}

describe('Downloads: the speed limit button', () => {
  it('says the stored limit: none, a step, and a value off the steps exactly', () => {
    for (const limit of [0, 20 * MB, 12 * MB, MB / 2]) {
      settings.set(stored(limit));
      const { unmount } = renderDom(Downloads);
      // After the settings are set: their language decides the words.
      const value = limit === 0 ? msg('downloads.page.limitNone') : rateLimitLabel(limit);
      expect(limitButton().textContent?.replace(/\s+/g, ' ').trim(), String(limit)).toBe(buttonName(value));
      expect(limitButton().getAttribute('aria-haspopup')).toBe('dialog');
      unmount();
    }
  });
});

describe('Downloads: the speed limit popover', () => {
  it('opens on the slider, which shows a value off the steps where it is and saves nothing until moved', async () => {
    await openLimit(12 * MB);

    expect(document.activeElement).toBe(slider());
    expect(slider().getAttribute('aria-valuetext')).toBe(rateLimitLabel(12 * MB));
    expect(Number(slider().getAttribute('aria-valuenow'))).toBeCloseTo(speedLimitPosition(12 * MB));
    expect(slider().getAttribute('aria-valuemax')).toBe(String(SPEED_LIMIT_NO_LIMIT));
    expect(saveSettingsPatch).not.toHaveBeenCalled();
  });

  it('a key commits the next step through the settings save, and the button says it', async () => {
    vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...stored(0), ...patch }) as Settings);
    const { user } = await openLimit(20 * MB);

    await user.keyboard('{ArrowRight}');

    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ downloadRateLimit: 30 * MB });
    await waitFor(() => expect(get(settings)?.downloadRateLimit).toBe(30 * MB));
    expect(limitButton().textContent).toContain(rateLimitLabel(30 * MB));
    expect(slider().getAttribute('aria-valuetext')).toBe(rateLimitLabel(30 * MB));
    expect(document.activeElement).toBe(slider());
  });

  it('End lifts the limit: the save sends 0', async () => {
    vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...stored(0), ...patch }) as Settings);
    const { user } = await openLimit(20 * MB);

    await user.keyboard('{End}');

    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ downloadRateLimit: 0 });
    await waitFor(() => expect(limitButton().textContent).toContain(msg('downloads.page.limitNone')));
  });

  it('a refused save puts the thumb back and says why by its code, in the popover', async () => {
    let refuse: (err: Error) => void = () => {};
    vi.mocked(saveSettingsPatch).mockImplementation(
      () =>
        new Promise<Settings>((_resolve, reject) => {
          refuse = reject;
        }),
    );
    const { user } = await openLimit(20 * MB);

    await user.keyboard('{ArrowRight}');
    // Busy while it saves, and still the focused control.
    expect(slider().getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(slider());

    refuse(new Error('typhon:settings.library_not_writable: denied'));

    await waitFor(() => expect(slider().getAttribute('aria-valuetext')).toBe(rateLimitLabel(20 * MB)));
    expect(slider().getAttribute('aria-valuenow')).toBe(String(speedLimitPosition(20 * MB)));
    await waitFor(() => expect(slider().hasAttribute('aria-busy')).toBe(false));
    expect(document.activeElement).toBe(slider());
    expect(limitButton().textContent).toContain(rateLimitLabel(20 * MB));
    const dialog = screen.getByRole('dialog', { name: msg('downloads.page.limitLabel') });
    const alert = within(dialog).getByRole('alert');
    expect(alert.textContent).toContain(msg('downloads.page.limitSaveFailed', { reason: msg('settings.library_not_writable') }));
    expect(get(toasts)).toEqual([]);
  });

  it('a drag shows each step on the slider and the button, and saves once on release', async () => {
    vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...stored(0), ...patch }) as Settings);
    await openLimit(20 * MB);
    const track = document.querySelector('.popover .track') as HTMLElement;
    const bar = document.querySelector('.popover .bar') as HTMLElement;
    const LEFT = 100;
    const WIDTH = 360;
    track.getBoundingClientRect = () => ({ left: LEFT, width: WIDTH, right: LEFT + WIDTH, top: 0, bottom: 4, height: 4, x: LEFT, y: 0, toJSON: () => ({}) });
    bar.setPointerCapture = () => {};
    bar.hasPointerCapture = () => false;
    const xOf = (step: number) => LEFT + (step / SPEED_LIMIT_NO_LIMIT) * WIDTH;

    await fireEvent.pointerDown(bar, { pointerId: 1, button: 0, clientX: xOf(2) });
    await fireEvent.pointerMove(bar, { pointerId: 1, clientX: xOf(3) });

    expect(slider().getAttribute('aria-valuetext')).toBe(rateLimitLabel(5 * MB));
    expect(limitButton().textContent).toContain(rateLimitLabel(5 * MB));
    expect(saveSettingsPatch).not.toHaveBeenCalled();

    await fireEvent.pointerUp(bar, { pointerId: 1, clientX: xOf(3) });

    expect(saveSettingsPatch).toHaveBeenCalledExactlyOnceWith({ downloadRateLimit: 5 * MB });
    await waitFor(() => expect(get(settings)?.downloadRateLimit).toBe(5 * MB));
  });

  it('Escape closes it and puts focus back on the button', async () => {
    const { user } = await openLimit(20 * MB);

    await user.keyboard('{Escape}');

    expect(screen.queryByRole('slider')).toBeNull();
    expect(document.activeElement).toBe(limitButton());
    expect(limitButton().getAttribute('aria-expanded')).toBe('false');
  });

  it('says where the finer control is and leads there', async () => {
    const { user } = await openLimit(20 * MB);
    const dialog = screen.getByRole('dialog', { name: msg('downloads.page.limitLabel') });

    await user.click(within(dialog).getByRole('button', { name: msg('downloads.page.limitSettings') }));

    expect(get(route)).toMatchObject({ name: 'settings', params: { tab: 'downloads' } });
  });
});
