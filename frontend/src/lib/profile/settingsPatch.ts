import type { ProfileSettings, ProfileSettingsPatch, Visibility } from '../services/account';

export function withFeedKind(hidden: string[] | null | undefined, kind: string, shown: boolean): string[] {
  const rest = (hidden ?? []).filter((item) => item !== kind);
  return shown ? rest : [...rest, kind];
}

export function feedHiddenChanged(draft: string[] | null | undefined, stored: string[] | null | undefined): boolean {
  const a = new Set(draft ?? []);
  const b = new Set(stored ?? []);
  return a.size !== b.size || [...a].some((kind) => !b.has(kind));
}

export function privacyPatch(
  draft: ProfileSettings,
  visibility: Visibility,
  stored?: ProfileSettings,
): ProfileSettingsPatch {
  const feedHidden = draft.feedHidden ?? [];
  const sendFeed = stored !== undefined && feedHiddenChanged(feedHidden, stored.feedHidden);
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
