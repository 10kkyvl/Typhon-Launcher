import type { CurrentUser } from '../services/account';
import type { PublicProfile } from '../services/social';
import { appearanceOf } from './appearance';
import { frameLook, type FrameProps } from './frameLook';

export function ownFrame(user?: CurrentUser | null): FrameProps {
  if (!user) return { frame: 'none' };
  const look = appearanceOf(user.profile?.appearance);
  return frameLook(look.avatarFrame, look.frameColor, user.frames);
}

export function publicFrame(profile?: PublicProfile | null): FrameProps {
  if (!profile) return { frame: 'none' };
  const look = appearanceOf(profile.appearance);
  return frameLook(look.avatarFrame, look.frameColor, profile.avatarFrameImage);
}
