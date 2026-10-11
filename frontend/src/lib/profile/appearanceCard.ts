import type { ProfileAppearance } from '../services/account';
import type { GameCard, PlayerCardStyle } from '../services/social';
import { appearanceAccentStyle, appearanceOf, appearancePalette, themeOf, type ProfileTheme } from './appearance';

export interface PlayerCardView {
  appearance: ProfileAppearance;
  theme: ProfileTheme;
  style: string;
  accent: string;
  statusEmoji: string;
  statusText: string;
  coverUrl: string;
  pinned: GameCard | null;
}

export function playerCardOf(card?: PlayerCardStyle | null): PlayerCardView {
  const appearance = appearanceOf({
    theme: card?.theme,
    accent: card?.accent,
    customFrom: card?.customFrom,
    customTo: card?.customTo,
    customAngle: card?.customAngle,
    avatarFrame: card?.avatarFrame,
    nameStyle: card?.nameStyle,
  });
  return {
    appearance,
    theme: themeOf(appearance),
    style: appearanceAccentStyle(appearance),
    accent: appearancePalette(appearance)['--accent'] ?? appearance.accent,
    statusEmoji: typeof card?.statusEmoji === 'string' ? card.statusEmoji : '',
    statusText: typeof card?.statusText === 'string' ? card.statusText : '',
    coverUrl: typeof card?.coverUrl === 'string' ? card.coverUrl : '',
    pinned: card?.pinned && typeof card.pinned.title === 'string' ? card.pinned : null,
  };
}
