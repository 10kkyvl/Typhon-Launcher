import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import { cardFrame, fieldsFrame, frameLook, frameScale } from './frameLook';
import { ownFrame, publicFrame } from './frameSources';
import { appearanceOf } from './appearance';
import { playerCardOf } from './appearanceCard';
import type { CurrentUser } from '../services/account';
import type { PublicProfile } from '../services/social';

const id = '3f2b8c1e-9a4d-4e57-b0aa-1c2d3e4f5a6b';
const image = { url: 'https://cdn/f.png', hole: 0.5 };
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
