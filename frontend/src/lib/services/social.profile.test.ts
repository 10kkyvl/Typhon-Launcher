import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = { Profile: vi.fn() };

vi.mock('../../../bindings/typhon/internal/social', () => ({ Service: bindings }));
vi.mock('./backend', () => ({ inWails: true }));

const base = { id: '1', username: 'alex', displayName: 'Alex', avatarUrl: '' };

describe('public profile mapping', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('keeps the layout, status, card and auto game the server sends', async () => {
    const { profile } = await import('./social');
    const layout = {
      version: 1,
      blocks: [{ id: 'b1', type: 'genres', width: 'half', config: {}, data: { genres: [{ name: 'RPG', share: 1 }], other: 0, unknown: 0 } }],
    };
    const autoGame = { igdbId: 7, title: 'Hades', coverUrl: 'c', heroUrl: 'h' };
    const card = { accent: '#ff0000', avatarFrame: 'neon', nameStyle: 'glow' };
    bindings.Profile.mockResolvedValue({ ...base, layout, statusEmoji: '🎮', statusText: 'farming', autoGame, card });

    const got = await profile('alex');

    expect(got.layout).toEqual(layout);
    expect(got.statusEmoji).toBe('🎮');
    expect(got.statusText).toBe('farming');
    expect(got.autoGame).toEqual(autoGame);
    expect(got.card).toEqual(card);
  });

  it('reads an old server without the new fields as no layout', async () => {
    const { profile } = await import('./social');
    bindings.Profile.mockResolvedValue({ ...base });

    const got = await profile('alex');

    expect(got.layout).toBeNull();
    expect(got.statusEmoji).toBe('');
    expect(got.autoGame).toBeNull();
    expect(got.card).toBeNull();
  });

  it('turns a null block list into an empty one', async () => {
    const { profile } = await import('./social');
    bindings.Profile.mockResolvedValue({ ...base, layout: { version: 1, blocks: null } });

    const got = await profile('alex');

    expect(got.layout).toEqual({ version: 1, blocks: [] });
  });
});

describe('chat peer from a user card', () => {
  const image = { url: 'https://cdn.test/f.webp', hole: 0.65 };

  it('takes the frame from the card of a friend list entry', async () => {
    const { chatPeerOf } = await import('./social');
    const card = { accent: '#ff0000', avatarFrame: 'custom:f1', frameImage: image, frameColor: '#00ff00', nameStyle: 'plain' } as const;

    expect(chatPeerOf({ ...base, card })).toEqual({ ...base, avatarFrame: 'custom:f1', frameColor: '#00ff00', frameImage: image });
  });

  it('falls back to the appearance and frame image of a public profile', async () => {
    const { chatPeerOf } = await import('./social');
    const appearance = { avatarFrame: 'custom:f1', frameColor: '' } as never;

    expect(chatPeerOf({ ...base, appearance, avatarFrameImage: image })).toMatchObject({ avatarFrame: 'custom:f1', frameImage: image });
  });

  it('leaves a user without a frame unframed', async () => {
    const { chatPeerOf } = await import('./social');

    expect(chatPeerOf(base)).toEqual({ ...base, avatarFrame: undefined, frameColor: undefined, frameImage: null });
  });
});
