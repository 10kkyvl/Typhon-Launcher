import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../i18n';
import { get } from 'svelte/store';
import { gameArt, initMetadata, loadArt, metadataAvailable, requestArt } from './metadata';

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => vi.fn()) },
}));

vi.mock('../services/backend', () => ({ inWails: true }));

const { ensureArt, getGameArt, isMetadataAvailable, toast } = vi.hoisted(() => ({
  ensureArt: vi.fn(),
  getGameArt: vi.fn(),
  isMetadataAvailable: vi.fn(async () => true),
  toast: vi.fn(),
}));

vi.mock('../services/metadata', () => ({ ensureArt, getGameArt, isMetadataAvailable }));
vi.mock('./toasts', () => ({ toast }));

let resets = 0;

async function forgetShownFailure() {
  resets += 1;
  await loadArt([`reset-${resets}`]);
  getGameArt.mockClear();
}

describe('metadata art pump error reporting', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    getGameArt.mockResolvedValue({});
    locale.set('en');
    await forgetShownFailure();
  });

  afterEach(() => {
    locale.set('ru');
  });

  it('shows the english translation of a known backend error code in the english locale', async () => {
    ensureArt.mockRejectedValue(new Error('typhon:metadata.not_configured: провайдер метаданных не настроен'));

    requestArt(['pump-error-known-code']);

    await vi.waitFor(() => expect(toast).toHaveBeenCalled());
    expect(toast).toHaveBeenCalledWith('The metadata provider is not configured', 'danger');
  });

  it('shows the translated generic fallback, not the raw message, for an error without a known code', async () => {
    ensureArt.mockRejectedValue(new Error('socket hang up'));

    requestArt(['pump-error-unknown-code']);

    await vi.waitFor(() => expect(toast).toHaveBeenCalled());
    expect(toast).toHaveBeenCalledWith('Failed to load cover art', 'danger');
  });

  it('reports a lasting failure once while a screen keeps asking for the same art', async () => {
    getGameArt.mockRejectedValue(new Error('socket hang up'));
    const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

    requestArt(['pump-spam-a']);
    await vi.waitFor(() => expect(toast).toHaveBeenCalledTimes(1));
    for (let tick = 0; tick < 5; tick += 1) {
      requestArt(['pump-spam-a']);
      await settle();
    }

    expect(toast).toHaveBeenCalledTimes(1);
    expect(getGameArt).toHaveBeenCalledTimes(1);
  });
});

describe('metadata availability and art loading errors', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    getGameArt.mockResolvedValue({});
    isMetadataAvailable.mockResolvedValue(true);
    locale.set('en');
    await forgetShownFailure();
  });

  afterEach(() => {
    locale.set('ru');
  });

  it('tells the user when availability could not be checked and keeps the metadata off', async () => {
    isMetadataAvailable.mockRejectedValueOnce(new Error('typhon:metadata.not_configured: провайдер метаданных не настроен'));

    await initMetadata();

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast).toHaveBeenCalledWith('The metadata provider is not configured', 'danger');
    expect(get(metadataAvailable)).toBe(false);
  });

  it('turns the metadata on when the check succeeds', async () => {
    await initMetadata();

    expect(toast).not.toHaveBeenCalled();
    expect(get(metadataAvailable)).toBe(true);
  });

  it('tells the user when art for a screen could not be loaded, without throwing into the screen', async () => {
    getGameArt.mockRejectedValueOnce(new Error('socket hang up'));

    await expect(loadArt(['art-error-a'])).resolves.toBeUndefined();

    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast).toHaveBeenCalledWith('Failed to load cover art', 'danger');
    expect(get(gameArt)['art-error-a']).toBeUndefined();
  });

  it('does not repeat the same failure on every reload of a screen, but reports a new one after a success', async () => {
    await loadArt(['art-repeat-start']);
    getGameArt.mockRejectedValue(new Error('socket hang up'));
    await loadArt(['art-repeat-a']);
    await loadArt(['art-repeat-b']);
    expect(toast).toHaveBeenCalledTimes(1);

    getGameArt.mockResolvedValueOnce({ 'art-repeat-c': { cover: 'c', hero: 'h' } });
    await loadArt(['art-repeat-c']);
    expect(get(gameArt)['art-repeat-c']).toEqual({ cover: 'c', hero: 'h' });

    await loadArt(['art-repeat-d']);
    expect(toast).toHaveBeenCalledTimes(2);
  });
});
