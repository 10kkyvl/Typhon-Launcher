import type { DownloadOrigin } from '../services/downloads';
import type { Release, ReleaseDownloadRequest } from '../services/sources';
import type { AvailabilityKind } from '../services/updates';

export function releaseOrigin(request: ReleaseDownloadRequest): DownloadOrigin {
  const origin: DownloadOrigin = {
    releaseId: request.releaseId,
    sourceId: request.sourceId,
    gameId: request.gameId,
  };
  if (request.version) origin.version = request.version;
  return origin;
}

export type ReleaseBadge = 'installed' | 'update' | 'new-release' | 'new' | 'none';

export interface ReleaseBadgeInput {
  releaseId: string;
  currentReleaseId: string;
  targetReleaseId: string;
  updateKind: AvailabilityKind;
  isNew: boolean;
}

export function releaseBadge(input: ReleaseBadgeInput): ReleaseBadge {
  if (input.currentReleaseId && input.releaseId === input.currentReleaseId) return 'installed';
  if (input.targetReleaseId && input.releaseId === input.targetReleaseId) {
    if (input.updateKind === 'update') return 'update';
    if (input.updateKind === 'new_release') return 'new-release';
  }
  return input.isNew ? 'new' : 'none';
}

export type BuildKind = 'portable' | 'repack' | 'none';

export function buildKind(release: Pick<Release, 'tags' | 'repacker'>): BuildKind {
  const tags = release.tags ?? [];
  if (tags.includes('portable')) return 'portable';
  if (release.repacker) return 'none';
  return tags.includes('repack') ? 'repack' : 'none';
}
