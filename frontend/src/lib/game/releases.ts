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
  | 'archive'
  | 'steam-rip'
  | 'gog'
  | 'license'
  | 'early-access'
  | 'demo'
  | 'p2p'
  | 'none';

// Порядок отвечает на вопрос «в каком виде это приедет»: форма поставки важнее
// происхождения файлов, а происхождение — важнее того, кто их выложил.
const buildOrder = [
  'portable',
  'archive',
  'steam-rip',
  'gog',
  'license',
  'early-access',
  'demo',
  'p2p',
] as const;

const buildLabels: Record<Exclude<BuildKind, 'none'>, MessageKey> = {
  portable: 'release.buildPortable',
  archive: 'release.buildArchive',
  'steam-rip': 'release.buildSteamRip',
  gog: 'release.buildGog',
  license: 'release.buildLicense',
  'early-access': 'release.buildEarlyAccess',
  demo: 'release.buildDemo',
  p2p: 'release.buildP2P',
};

// Слаг уезжает на сервер общей статистики и потому латинский, а подписаны
// сборщики так, как их знают в раздачах.
const repackerLabels: Record<string, string> = {
  fitgirl: 'FitGirl',
  elamigos: 'ElAmigos',
  dodi: 'DODI',
  kaoskrew: 'KaosKrew',
  mechanics: 'Механики',
  elementarts: 'Element Arts',
  rggames: 'R.G. Games',
  blackbeard: 'Black Beard',
  masterdarkness: 'MasterDarkness',
  mop030b: 'MOP030B',
  spacex: 'SpaceX',
  xlaser: 'XLASER',
  seyter: 'SEYTER',
  vicknet: 'VickNet',
  recoding: 'ReCoding',
  unigamers: 'UniGamers',
  revolution: 'REVOLUTiON',
  armeniac: 'ARMENIAC',
  others: "Other's",
  nemos: 'nemos',
  qoob: 'qoob',
  z10yded: 'z10yded',
  dixen18: 'dixen18',
};

export function buildKind(release: Pick<Release, 'tags'>): BuildKind {
  const tags = release.tags ?? [];
  for (const kind of buildOrder) {
    if (tags.includes(kind)) return kind;
  }
  return 'none';
}

export function buildLabel(kind: BuildKind): MessageKey | null {
  return kind === 'none' ? null : buildLabels[kind];
}

export function repackerLabel(slug: string): string {
  return repackerLabels[slug] ?? slug.charAt(0).toUpperCase() + slug.slice(1);
}

// Приписка к источнику: кто собрал раздачу. Отдельно от формы поставки, потому
// что «репак от Xatab» отвечает на другой вопрос, чем «портативная» — и, в
// отличие от неё, привязан к тому, откуда раздача взялась.
export function repackNote(release: Pick<Release, 'tags' | 'repacker'>, repackWord: string): string {
  const name = release.repacker ? repackerLabel(release.repacker) : '';
  if ((release.tags ?? []).includes('repack')) return name ? `${repackWord} ${name}` : repackWord;
  return name;
}
