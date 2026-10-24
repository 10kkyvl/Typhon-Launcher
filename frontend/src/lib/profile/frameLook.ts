import { AVATAR_FRAMES, type AvatarFrame, type AvatarPreset, type FrameImage, type OwnedFrame } from '../services/account';
import type { PlayerCardStyle } from '../services/social';
import { validAccent } from '../theme/accent';

const CUSTOM = /^custom:([0-9a-f-]{36})$/;
const MIN_HOLE = 0.3;

export interface FrameProps {
  frame: AvatarFrame;
  frameColor?: string;
  frameImage?: FrameImage;
}

export function isCustomFrame(value: unknown): value is `custom:${string}` {
  return typeof value === 'string' && CUSTOM.test(value);
}

export function customFrameId(value: unknown): string {
  return typeof value === 'string' ? (CUSTOM.exec(value)?.[1] ?? '') : '';
}

export function presetFrame(value: unknown): AvatarPreset {
  return typeof value === 'string' && (AVATAR_FRAMES as readonly string[]).includes(value) ? (value as AvatarPreset) : 'none';
}

export function frameScale(hole: number): number {
  const clamped = Math.min(1, Math.max(MIN_HOLE, hole));
  return Math.round((100 / clamped) * 100) / 100;
}

function usable(image?: FrameImage | null): FrameImage | undefined {
  if (!image || typeof image.url !== 'string' || image.url === '') return undefined;
  if (typeof image.hole !== 'number' || !Number.isFinite(image.hole) || image.hole <= 0) return undefined;
  return { url: image.url, hole: image.hole };
}

export function ownedFrame(owned: readonly OwnedFrame[] | null | undefined, frame: unknown): OwnedFrame | undefined {
  const id = customFrameId(frame);
  return id ? owned?.find((item) => item.id === id) : undefined;
}

export function frameLook(
  frame: unknown,
  color: string | undefined,
  source?: FrameImage | readonly OwnedFrame[] | null,
): FrameProps {
  if (isCustomFrame(frame)) {
    const image = usable(Array.isArray(source) ? ownedFrame(source as readonly OwnedFrame[], frame) : (source as FrameImage | null | undefined));
    return image ? { frame, frameImage: image } : { frame: 'none' };
  }
  const preset = presetFrame(frame);
  if (preset === 'none') return { frame: 'none' };
  return { frame: preset, frameColor: typeof color === 'string' && validAccent(color) ? color : undefined };
}

interface FrameFields {
  avatarFrame?: string;
  frameColor?: string;
  frameImage?: FrameImage | null;
}

export function fieldsFrame(source?: FrameFields | null): FrameProps {
  return frameLook(source?.avatarFrame, source?.frameColor, source?.frameImage);
}

export function cardFrame(card?: PlayerCardStyle | null): FrameProps {
  if (!card) return { frame: 'none' };
  const color = [card.frameColor, card.accent].find((value) => typeof value === 'string' && validAccent(value));
  return frameLook(card.avatarFrame, color, card.frameImage);
}
