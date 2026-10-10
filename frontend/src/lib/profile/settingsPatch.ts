import type { ProfileSettings, Visibility } from '../services/account';

export function privacyPatch(draft: ProfileSettings, visibility: Visibility): ProfileSettings {
  return {
    visibility,
    showOnline: draft.showOnline,
    showPlaying: draft.showPlaying,
    showPlaytime: draft.showPlaytime,
    showLibrary: draft.showLibrary,
    showActivity: draft.showActivity,
    showStats: draft.showStats,
    showcase: [...draft.showcase],
    appearance: draft.appearance,
  };
}
