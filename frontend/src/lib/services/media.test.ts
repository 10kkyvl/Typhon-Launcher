import { beforeEach, describe, expect, it, vi } from 'vitest';

const media = {
  Current: vi.fn(),
};

vi.mock('../../../bindings/typhon/internal/media', () => ({ Service: media }));

async function loadMedia(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  return import('./media');
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe('media state', () => {
  it('reads a full state', async () => {
    const mediaModule = await loadMedia(true);
    const track = { app: 'Spotify', title: 'T', artist: 'A', album: 'B', playing: true, canPlayPause: true, canNext: true, canPrev: false };

    expect(mediaModule.toMediaState({ supported: true, active: true, track })).toEqual({
      supported: true,
      active: true,
      track,
    });
  });

  it('turns missing or malformed pieces into an inactive, empty state', async () => {
    const mediaModule = await loadMedia(true);
    const empty = {
      supported: false,
      active: false,
      track: { app: '', title: '', artist: '', album: '', playing: false, canPlayPause: false, canNext: false, canPrev: false },
    };

    expect(mediaModule.toMediaState(null)).toEqual(empty);
    expect(mediaModule.toMediaState({})).toEqual(empty);
    expect(mediaModule.toMediaState({ supported: 'yes', track: { playing: 'yes', title: null } })).toEqual(empty);
  });

  it('recognises the unavailable code and nothing else', async () => {
    const mediaModule = await loadMedia(true);

    expect(mediaModule.isMediaUnavailable(new Error('typhon:media.unavailable: no session manager'))).toBe(true);
    expect(mediaModule.isMediaUnavailable(new Error('typhon:media.other: x'))).toBe(false);
    expect(mediaModule.isMediaUnavailable(new Error('media unavailable'))).toBe(false);
  });

  it('reports an unsupported player in a browser preview and calls the backend inside the app', async () => {
    media.Current.mockResolvedValue({ supported: true, active: false, track: {} });

    const preview = await loadMedia(false);
    await expect(preview.currentMedia()).resolves.toMatchObject({ supported: false, active: false });
    expect(media.Current).not.toHaveBeenCalled();

    const app = await loadMedia(true);
    await expect(app.currentMedia()).resolves.toMatchObject({ supported: true, active: false });
    expect(media.Current).toHaveBeenCalledTimes(1);
  });
});
