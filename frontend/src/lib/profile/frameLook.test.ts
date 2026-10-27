import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import { MAX_FRAME_BLEED, cardFrame, fieldsFrame, frameBleed, frameCenter, frameLook, frameOverhang, frameReserve, frameScale, frameShift } from './frameLook';
import { ownFrame, publicFrame } from './frameSources';
import { appearanceOf } from './appearance';
import { playerCardOf } from './appearanceCard';
import type { CurrentUser } from '../services/account';
import type { PublicProfile } from '../services/social';

const id = '3f2b8c1e-9a4d-4e57-b0aa-1c2d3e4f5a6b';
const image = { url: 'https://cdn/f.png', hole: 0.5, holeX: 0.5, holeY: 0.5 };
const owned = [{ id, name: 'Dachshund', ...image }];

describe('frameLook', () => {
  it('passes a custom frame with its image and no colour', () => {
    expect(frameLook(`custom:${id}`, '#ff0000', image)).toEqual({ frame: `custom:${id}`, frameImage: image });
  });
  it('treats a custom frame without an image as none', () => {
    expect(frameLook(`custom:${id}`, '#ff0000')).toEqual({ frame: 'none' });
    expect(frameLook(`custom:${id}`, '', null)).toEqual({ frame: 'none' });
    expect(frameLook(`custom:${id}`, '', { url: '', hole: 0.5 })).toEqual({ frame: 'none' });
    expect(frameLook(`custom:${id}`, '', { url: 'u', hole: 0 })).toEqual({ frame: 'none' });
  });
  it('keeps a preset with its colour and drops an unknown one', () => {
    expect(frameLook('neon', '#00ff00', image)).toEqual({ frame: 'neon', frameColor: '#00ff00' });
    expect(frameLook('neon', '')).toEqual({ frame: 'neon', frameColor: undefined });
    expect(frameLook('rainbow', '#00ff00')).toEqual({ frame: 'none' });
    expect(frameLook('none', '#00ff00')).toEqual({ frame: 'none' });
    expect(frameLook(undefined, undefined)).toEqual({ frame: 'none' });
  });
  it('drops a frame colour that is not a hex colour', () => {
    for (const color of ['red; background: url(https://x/y)', 'url(https://x/y)', 'var(--accent)', '#12345']) {
      expect(frameLook('ring', color)).toEqual({ frame: 'ring', frameColor: undefined });
      expect(fieldsFrame({ avatarFrame: 'ring', frameColor: color })).toEqual({ frame: 'ring', frameColor: undefined });
    }
  });
  it('finds the image of an owned frame by id', () => {
    expect(frameLook(`custom:${id}`, '', owned)).toEqual({ frame: `custom:${id}`, frameImage: image });
  });
  it('shows a revoked own frame as none', () => {
    expect(frameLook(`custom:${id}`, '', [])).toEqual({ frame: 'none' });
    expect(frameLook(`custom:${id}`, '', null)).toEqual({ frame: 'none' });
    expect(frameLook(`custom:${id.replace('3f', '4f')}`, '', owned)).toEqual({ frame: 'none' });
  });
  it('scales the image by the hole and clamps it', () => {
    expect(frameScale(0.5)).toBe(200);
    expect(frameScale(1)).toBe(100);
    expect(frameScale(2)).toBe(100);
    expect(frameScale(0.01)).toBeCloseTo(333.33, 2);
  });
});

describe('frame hole centre', () => {
  const raw = { url: 'https://cdn/f.png', hole: 0.5 };
  it('defaults a missing centre to the middle', () => {
    expect(frameLook(`custom:${id}`, '', raw)).toEqual({ frame: `custom:${id}`, frameImage: image });
  });
  it('keeps an offset centre', () => {
    expect(frameLook(`custom:${id}`, '', { ...raw, holeX: 0.4, holeY: 0.62 })).toEqual({
      frame: `custom:${id}`,
      frameImage: { ...raw, holeX: 0.4, holeY: 0.62 },
    });
  });
  it('clamps and repairs a bad centre', () => {
    expect(frameCenter(0)).toBe(0.05);
    expect(frameCenter(2)).toBe(0.95);
    expect(frameCenter(Number.NaN)).toBe(0.5);
    expect(frameCenter(undefined)).toBe(0.5);
    expect(frameCenter('0.3')).toBe(0.5);
  });
  it('turns a centre into a negative shift', () => {
    expect(frameShift(0.5)).toBe('-50%');
    expect(frameShift(0.4)).toBe('-40%');
    expect(frameShift(0.6234)).toBe('-62.34%');
  });

  it('reserves the overhang on each side from the hole centre', () => {
    expect(frameOverhang(0.5, 0.5)).toBe(0.5);
    expect(frameOverhang(0.5, 0.25)).toBe(0);
    expect(frameOverhang(0.5, 0.75)).toBe(1);
    expect(frameOverhang(1, 0.5)).toBe(0);
  });
});

describe('frame sources', () => {
  it('reads message and review fields', () => {
    expect(fieldsFrame({ avatarFrame: `custom:${id}`, frameColor: '', frameImage: image })).toEqual({ frame: `custom:${id}`, frameImage: image });
    expect(fieldsFrame({ avatarFrame: `custom:${id}`, frameColor: '' })).toEqual({ frame: 'none' });
    expect(fieldsFrame(null)).toEqual({ frame: 'none' });
  });
  it('reads a player card and falls back to its accent for the colour', () => {
    const card = { accent: '#112233', avatarFrame: 'ring' as const, nameStyle: 'plain' as const };
    expect(cardFrame(card)).toEqual({ frame: 'ring', frameColor: '#112233' });
    expect(cardFrame({ ...card, frameColor: '#aabbcc' })).toEqual({ frame: 'ring', frameColor: '#aabbcc' });
    expect(cardFrame({ ...card, avatarFrame: `custom:${id}`, frameImage: image })).toEqual({ frame: `custom:${id}`, frameImage: image });
    expect(cardFrame(undefined)).toEqual({ frame: 'none' });
  });
  it('takes the own image from the granted frames', () => {
    const user = { profile: { appearance: { avatarFrame: `custom:${id}` } }, frames: owned } as unknown as CurrentUser;
    expect(ownFrame(user)).toEqual({ frame: `custom:${id}`, frameImage: image });
    expect(ownFrame({ ...user, frames: null })).toEqual({ frame: 'none' });
    expect(ownFrame(null)).toEqual({ frame: 'none' });
  });
  it('takes the public image from the profile', () => {
    const profile = { appearance: { avatarFrame: `custom:${id}` }, avatarFrameImage: image } as unknown as PublicProfile;
    expect(publicFrame(profile)).toEqual({ frame: `custom:${id}`, frameImage: image });
    expect(publicFrame({ ...profile, avatarFrameImage: null })).toEqual({ frame: 'none' });
  });
});

describe('custom frame normalisation', () => {
  it('keeps a lowercase custom id in the appearance and the player card', () => {
    expect(appearanceOf({ avatarFrame: `custom:${id}` }).avatarFrame).toBe(`custom:${id}`);
    const card = { accent: '#ff8800', nameStyle: 'plain' as const, avatarFrame: `custom:${id}` as const, frameImage: image };
    const view = playerCardOf(card);
    expect(view.appearance.avatarFrame).toBe(`custom:${id}`);
    expect(view.frameImage).toEqual(image);
  });
  it.each(['custom:bad', `CUSTOM:${id}`, `custom:${id.toUpperCase()}`, `custom:${id}x`, 'custom:', 'custom'])(
    'drops %s',
    (value) => {
      expect(appearanceOf({ avatarFrame: value as never }).avatarFrame).toBe('none');
      expect(playerCardOf({ accent: '#ff8800', nameStyle: 'plain', avatarFrame: value as never }).appearance.avatarFrame).toBe('none');
    },
  );
  it('gives the card view no image for a custom frame without one', () => {
    const card = { accent: '#ff8800', nameStyle: 'plain' as const, avatarFrame: `custom:${id}` as const };
    expect(playerCardOf(card).frameImage).toBeUndefined();
  });
});

describe('frameBleed', () => {
  it('equals the overhang while it stays within the bound', () => {
    expect(frameBleed(0.8, 0.5)).toBeCloseTo(0.125, 5);
    expect(frameBleed(0.5, 0.25)).toBe(0);
    expect(frameBleed(1, 0.5)).toBe(0);
  });
  it('caps the extreme frames the server accepts at the maximum bleed', () => {
    expect(MAX_FRAME_BLEED).toBe(0.5);
    expect(frameOverhang(0.3, 0.95)).toBeGreaterThan(2.6);
    expect(frameBleed(0.3, 0.95)).toBe(MAX_FRAME_BLEED);
    expect(frameBleed(0.3, 0.05)).toBe(0);
  });
  it('scales the span by the image aspect ratio', () => {
    expect(frameBleed(0.8, 0.5, 1)).toBeCloseTo(0.125, 5);
    expect(frameBleed(0.8, 0.5, 1.2)).toBeCloseTo(0.25, 5);
    expect(frameBleed(0.8, 0.5, 2)).toBe(MAX_FRAME_BLEED);
    expect(frameBleed(0.8, 0.5, 0)).toBeCloseTo(frameBleed(0.8, 0.5, 1), 5);
    expect(frameBleed(0.8, 0.5, Number.NaN)).toBeCloseTo(frameBleed(0.8, 0.5, 1), 5);
  });
});

describe('frameReserve', () => {
  const frame = 'custom:3f2b8c1e-9a4d-4e57-b0aa-1c2d3e4f5a6b';

  it('reserves nothing for a list without personal frames', () => {
    expect(frameReserve([])).toEqual({ left: 0, right: 0 });
    expect(frameReserve([{ frame: 'none' }, { frame: 'ring', frameColor: '#ff8800' }])).toEqual({ left: 0, right: 0 });
  });

  it('takes the widest overhang on each side across the list', () => {
    const left = { frame, frameImage: { url: 'a', hole: 0.8, holeX: 0.6, holeY: 0.5 } } as const;
    const right = { frame, frameImage: { url: 'b', hole: 0.8, holeX: 0.45, holeY: 0.5 } } as const;
    const reserve = frameReserve([{ frame: 'none' }, left, right]);
    expect(reserve.left).toBeCloseTo(0.25, 5);
    expect(reserve.right).toBeCloseTo(0.1875, 5);
  });

  it('never reserves more than the bleed cap', () => {
    const wide = { frame, frameImage: { url: 'a', hole: 0.3, holeX: 0.95, holeY: 0.5 } } as const;
    expect(frameReserve([wide]).left).toBe(MAX_FRAME_BLEED);
  });

  it('ignores an image on a preset or a custom frame without an image', () => {
    expect(frameReserve([{ frame: 'ring', frameImage: { url: 'a', hole: 0.3 } }, { frame }])).toEqual({ left: 0, right: 0 });
  });
});
