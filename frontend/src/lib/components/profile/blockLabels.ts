import { msg } from '../../i18n';
import type { ProfileKey } from '../../i18n/catalog/ru/profile';
import { isKnownType } from '../../profile/layout';
import type { BlockType } from '../../services/account';

const LABELS: Record<BlockType, ProfileKey> = {
  pinned: 'profile.blockPinned',
  collection: 'profile.blockCollection',
  genres: 'profile.blockGenres',
  fingerprint: 'profile.blockFingerprint',
  text: 'profile.blockText',
  recent: 'profile.blockRecent',
  activity: 'profile.blockActivity',
  stats: 'profile.blockStats',
  playing: 'profile.blockPlaying',
  about: 'profile.blockAbout',
};

export function blockLabel(type: string): string {
  return isKnownType(type) ? msg(LABELS[type]) : '';
}
