import { Service as ReviewsService } from '../../../bindings/typhon/internal/reviews';
import type {
  Author,
  Eligibility,
  Limits,
  Mine,
  Page as RawPage,
  Review,
  Summary,
  VoteResult,
} from '../../../bindings/typhon/internal/reviews';
import { AccountError, toAccountError } from './account';
import { inWails } from './backend';

export type { Author, Eligibility, Limits, Mine, Review, Summary, VoteResult };

export interface Page {
  summary: Summary;
  reviews: Review[];
  next: string;
}

export type ReviewSort = 'helpful' | 'recent';
export type ReviewFilter = 'all' | 'positive' | 'negative';
export type ReviewVote = 'helpful' | 'unhelpful' | '';
export type ReportReason = 'spam' | 'offensive' | 'spoilers' | 'off_topic' | 'other';

const unauthenticated = () => new AccountError('unauthenticated');

export function emptyPage(): Page {
  return { summary: { total: 0, positive: 0 }, reviews: [], next: '' };
}

export function emptyMine(): Mine {
  return {
    review: null,
    eligibility: { canPost: false, reason: '', retryAt: '', playtimeSeconds: 0, requiredPlaytimeSeconds: 0 },
  };
}

function toPage(value: unknown): Page {
  const page = value as Partial<RawPage> | null;
  if (!page) return emptyPage();
  return {
    summary: page.summary ?? { total: 0, positive: 0 },
    reviews: page.reviews ?? [],
    next: page.next ?? '',
  };
}

function toMine(value: unknown): Mine {
  const result = value as Partial<Mine> | null;
  if (!result) return emptyMine();
  return {
    review: result.review ?? null,
    eligibility: result.eligibility ?? emptyMine().eligibility,
  };
}

export async function limits(): Promise<Limits> {
  if (!inWails) throw unauthenticated();
  try {
    return await ReviewsService.Limits();
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function list(
  canonicalGameId: string,
  sort: ReviewSort,
  filter: ReviewFilter,
  cursor = '',
): Promise<Page> {
  if (!inWails) return emptyPage();
  try {
    return toPage(await ReviewsService.List(canonicalGameId, sort, filter, cursor));
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function mine(canonicalGameId: string): Promise<Mine> {
  if (!inWails) return emptyMine();
  try {
    return toMine(await ReviewsService.Mine(canonicalGameId));
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function save(canonicalGameId: string, recommended: boolean, body: string): Promise<Review> {
  if (!inWails) throw unauthenticated();
  try {
    return await ReviewsService.Save(canonicalGameId, recommended, body);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function remove(canonicalGameId: string): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await ReviewsService.Delete(canonicalGameId);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function vote(reviewId: number, value: ReviewVote): Promise<VoteResult> {
  if (!inWails) throw unauthenticated();
  try {
    return await ReviewsService.Vote(reviewId, value);
  } catch (err) {
    throw toAccountError(err);
  }
}

export async function report(reviewId: number, reason: ReportReason): Promise<void> {
  if (!inWails) throw unauthenticated();
  try {
    await ReviewsService.Report(reviewId, reason);
  } catch (err) {
    throw toAccountError(err);
  }
}
