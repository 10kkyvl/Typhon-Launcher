import type { ProfileSettings, ProfileSettingsPatch, Visibility } from '../services/account';

export function withFeedKind(hidden: string[] | null | undefined, kind: string, shown: boolean): string[] {
  const rest = (hidden ?? []).filter((item) => item !== kind);
  return shown ? rest : [...rest, kind];
}

export function privacyPatch(draft: ProfileSettings, visibility: Visibility, feedTouched = false): ProfileSettingsPatch {
  const feedHidden = draft.feedHidden ?? [];
  const sendFeed = feedTouched || feedHidden.length > 0;
  return {
    visibility,
    showOnline: draft.showOnline,
    showPlaying: draft.showPlaying,
    showPlaytime: draft.showPlaytime,
    showLibrary: draft.showLibrary,
    showActivity: draft.showActivity,
    showStats: draft.showStats,
    ...(sendFeed ? { feedHidden: [...feedHidden] } : {}),
  };
}
