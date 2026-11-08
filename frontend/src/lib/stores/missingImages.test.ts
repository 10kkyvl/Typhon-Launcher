import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../services/emulators', () => ({ missingImages: vi.fn() }));

const { toast } = await import('./toasts');
const { missingImages: fetchMissing } = await import('../services/emulators');
const { missingImages, refreshMissingImages } = await import('./missingImages');
const { ru } = await import('../i18n/catalog/ru');

describe('refreshMissingImages', () => {
  beforeEach(async () => {
    vi.mocked(fetchMissing).mockReset();
    vi.mocked(fetchMissing).mockResolvedValue([]);
    await refreshMissingImages();
    vi.mocked(toast).mockReset();
    vi.mocked(fetchMissing).mockReset();
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  it('replaces the set on success', async () => {
    vi.mocked(fetchMissing).mockResolvedValue(['a', 'b']);
    await refreshMissingImages();
    expect([...get(missingImages)]).toEqual(['a', 'b']);
  });

  it('keeps the previous set and shows a toast when the check fails', async () => {
    vi.mocked(fetchMissing).mockResolvedValue(['a']);
    await refreshMissingImages();
    vi.mocked(fetchMissing).mockRejectedValue(new Error('boom'));
    await refreshMissingImages();
    expect([...get(missingImages)]).toEqual(['a']);
    expect(toast).toHaveBeenCalledWith(ru['emul.missingCheckFailed'], 'danger');
  });

  it('shows the toast once per run of failures and again after a success', async () => {
    vi.mocked(fetchMissing).mockRejectedValue(new Error('boom'));
    await refreshMissingImages();
    await refreshMissingImages();
    expect(toast).toHaveBeenCalledTimes(1);
    vi.mocked(fetchMissing).mockResolvedValue([]);
    await refreshMissingImages();
    vi.mocked(fetchMissing).mockRejectedValue(new Error('boom'));
    await refreshMissingImages();
    expect(toast).toHaveBeenCalledTimes(2);
  });
});
