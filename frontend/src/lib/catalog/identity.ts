import type { CatalogGame } from '../services/sources';

type IdentityGame = Pick<CatalogGame, 'id' | 'serverId' | 'externalIds' | 'providerLinks' | 'aliasIds'>;

function sortedStrings(values: string[] | undefined): string[] {
  return [...new Set((values ?? []).filter(Boolean))].sort();
}

function sortedRecord(record: Record<string, string | string[] | undefined> | undefined) {
  return Object.entries(record ?? {})
    .map(([key, values]) => [
      key,
      Array.isArray(values) ? sortedStrings(values) : values || '',
    ] as const)
    .sort(([left], [right]) => left.localeCompare(right));
}

function providerEvidence(game: IdentityGame): Set<string> {
  const evidence = new Set<string>();
  for (const [provider, value] of Object.entries(game.externalIds ?? {})) {
    if (value) evidence.add(`${provider}:${value}`);
  }
  for (const [provider, values] of Object.entries(game.providerLinks ?? {})) {
    for (const value of values ?? []) if (value) evidence.add(`${provider}:${value}`);
  }
  return evidence;
}

// Identity evidence is deliberately limited to provider and canonical links.
// Titles and metadata fields are mutable content and must not trigger a catalog
// reload or make two similarly named games look like one game.
export function identityFingerprint(game: IdentityGame | null | undefined): string {
  if (!game) return '';
  return JSON.stringify({
    id: game.id || '',
    serverId: game.serverId || '',
    externalIds: sortedRecord(game.externalIds),
    providerLinks: sortedRecord(game.providerLinks),
    aliasIds: sortedStrings(game.aliasIds),
  });
}

export function identityEvidenceChanged(before: IdentityGame | null | undefined, after: IdentityGame | null | undefined): boolean {
  return identityFingerprint(before) !== identityFingerprint(after);
}

export function matchesCatalogIdentity(item: IdentityGame, update: IdentityGame): boolean {
  if (item.id && item.id === update.id) return true;
  if (item.serverId && update.serverId && item.serverId === update.serverId) return true;
  if (item.aliasIds?.includes(update.id) || update.aliasIds?.includes(item.id)) return true;
  const itemEvidence = providerEvidence(item);
  if (itemEvidence.size === 0) return false;
  for (const evidence of providerEvidence(update)) {
    if (itemEvidence.has(evidence)) return true;
  }
  return false;
}
