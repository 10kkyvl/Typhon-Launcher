import type { SimilarRelease } from '../services/sources';

export interface CandidateNote {
  title: string;
  year: number;
}

export function relativeCandidate(item: Pick<SimilarRelease, 'matchedCandidate'>, gameId: string): CandidateNote | null {
  const candidate = item.matchedCandidate;
  if (!candidate || !candidate.id || candidate.id === gameId) return null;
  return { title: candidate.title, year: candidate.year > 0 ? candidate.year : 0 };
}

export function candidateLabel(note: CandidateNote): string {
  return note.year > 0 ? `${note.title} ${note.year}` : note.title;
}

export function showSimilar(input: {
  releasesLoading: boolean;
  releasesFailed: boolean;
  releaseCount: number;
  relatedSettled: boolean;
  hasGameId: boolean;
}): boolean {
  return input.hasGameId && input.relatedSettled && !input.releasesLoading && !input.releasesFailed && input.releaseCount === 0;
}

export function familyIds(items: { game: { id: string } }[]): string[] {
  return items.map((item) => item.game.id).filter(Boolean);
}

export function candidateGroups<T extends { source?: string }>(list: T[]): { server: T[]; local: T[] } {
  return {
    server: list.filter((item) => item.source === 'server'),
    local: list.filter((item) => item.source !== 'server'),
  };
}
