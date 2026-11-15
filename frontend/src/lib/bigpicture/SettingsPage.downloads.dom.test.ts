import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { getSettings, saveSettingsPatch, type Settings } from '../services/settings';
import { settings } from '../stores/settings';
import { renderDom } from '../testing/dom';
import SettingsPage from './components/SettingsPage.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/settings', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/settings')>()),
  saveSettingsPatch: vi.fn(),
}));

let stored: Settings;

beforeEach(async () => {
  stored = { ...(await getSettings()), language: 'en' };
  vi.mocked(saveSettingsPatch).mockImplementation(async (patch) => ({ ...stored, ...patch }));
});

afterEach(() => {
  settings.set(null);
  vi.mocked(saveSettingsPatch).mockReset();
});

function choices(): HTMLButtonElement[] {
  const row = screen.getByLabelText(msg('bp.settings.sectionDownloads'));
  return within(row).getAllByRole('button');
}

function label(count: number): string {
  return msg('bp.settings.downloadsValue', { count });
}

describe('Big Picture settings: downloads at a time', () => {
  it('shows a value the desktop stored as a fifth choice in its place, marked as the current one', () => {
    settings.set({ ...stored, maxActiveDownloads: 4 });
    renderDom(SettingsPage, { onback: vi.fn(), requestText: vi.fn(async () => null) });

    expect(choices().map((button) => button.textContent?.trim())).toEqual([1, 2, 3, 4, 5].map(label));
    expect(choices().filter((button) => button.getAttribute('aria-pressed') === 'true').map((button) => button.textContent?.trim())).toEqual([label(4)]);
  });

  it('puts a stored value above the four at the end and marks it', () => {
    settings.set({ ...stored, maxActiveDownloads: 8 });
    renderDom(SettingsPage, { onback: vi.fn(), requestText: vi.fn(async () => null) });

    expect(choices().map((button) => button.textContent?.trim())).toEqual([1, 2, 3, 5, 8].map(label));
    expect(screen.getByRole('button', { name: label(8) }).getAttribute('aria-pressed')).toBe('true');
  });

  it('offers the four alone when one of them is stored, and a press saves the one pressed', async () => {
    settings.set({ ...stored, maxActiveDownloads: 2 });
    const { user } = renderDom(SettingsPage, { onback: vi.fn(), requestText: vi.fn(async () => null) });

    expect(choices().map((button) => button.textContent?.trim())).toEqual([1, 2, 3, 5].map(label));
    expect(screen.getByRole('button', { name: label(2) }).getAttribute('aria-pressed')).toBe('true');

    await user.click(screen.getByRole('button', { name: label(5) }));
    expect(saveSettingsPatch).toHaveBeenCalledWith({ maxActiveDownloads: 5 });
    await vi.waitFor(() => expect(get(settings)?.maxActiveDownloads).toBe(5));
  });
});
