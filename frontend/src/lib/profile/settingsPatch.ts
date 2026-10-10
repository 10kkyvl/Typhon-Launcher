import type { ProfileSettings, ProfileSettingsPatch, Visibility } from '../services/account';

export function privacyPatch(draft: ProfileSettings, visibility: Visibility): ProfileSettingsPatch {
  return {
    visibility,
    showOnline: draft.showOnline,
    showPlaying: draft.showPlaying,
    showPlaytime: draft.showPlaytime,
    showLibrary: draft.showLibrary,
    showActivity: draft.showActivity,
    showStats: draft.showStats,
  };
}
