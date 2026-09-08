import type { MessageKey } from '../i18n';
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

export type BuildKind =
  | 'portable'
  | 'repack'
  | 'steam-rip'
  | 'gog'
  | 'license'
  | 'early-access'
  | 'demo'
  | 'p2p'
  | 'archive'
  | 'none';

// Порядок отвечает на вопрос «чем эта раздача отличается от соседней»: форма
// поставки важнее способа упаковки, способ упаковки важнее происхождения файлов.
const buildOrder = [
  'portable',
  'repack',
  'steam-rip',
  'gog',
  'license',
  'early-access',
  'demo',
  'p2p',
  'archive',
] as const;

const buildLabels: Record<Exclude<BuildKind, 'none'>, MessageKey> = {
  portable: 'release.buildPortable',
  repack: 'release.buildRepack',
  'steam-rip': 'release.buildSteamRip',
  gog: 'release.buildGog',
  license: 'release.buildLicense',
  'early-access': 'release.buildEarlyAccess',
  demo: 'release.buildDemo',
  p2p: 'release.buildP2P',
  archive: 'release.buildArchive',
};

// Слаг уезжает на сервер общей статистики и потому латинский, а подписаны
// сборщики так, как их знают в раздачах. Остальным хватает слага в верхнем
// регистре.
const repackerLabels: Record<string, string> = {
  mechanics: 'МЕХАНИКИ',
  elementarts: 'ELEMENT ARTS',
  rggames: 'R.G. GAMES',
  blackbeard: 'BLACK BEARD',
};

export function buildKind(release: Pick<Release, 'tags' | 'repacker'>): BuildKind {
  const tags = release.tags ?? [];
  for (const kind of buildOrder) {
    if (!tags.includes(kind)) continue;
    // Имя сборщика говорит то же самое и точнее, рядом с ним общий «Репак» лишний.
    if (kind === 'repack' && release.repacker) return 'none';
    return kind;
  }
  return 'none';
}

export function buildLabel(kind: BuildKind): MessageKey | null {
  return kind === 'none' ? null : buildLabels[kind];
}

export function repackerLabel(slug: string): string {
  return repackerLabels[slug] ?? slug.toUpperCase();
}
