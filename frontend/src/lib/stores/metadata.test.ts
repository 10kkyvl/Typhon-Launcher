import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../i18n';
import { requestArt } from './metadata';

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

describe('metadata art pump error reporting', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getGameArt.mockResolvedValue({});
    locale.set('en');
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
});
