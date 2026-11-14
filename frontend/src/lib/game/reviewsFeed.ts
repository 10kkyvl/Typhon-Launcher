import { get } from 'svelte/store';
import { list, type Page, type ReviewFilter, type ReviewSort } from '../services/reviews';
import { reviewsChanged } from './reviewsRefresh';

// The summary and the list ask for the same first page on one page open and again after a change of
// reviews: a request still in flight is shared, so the service is asked once. An answer is not kept:
// an ask after it has settled (a retry, the next open) is a new request. A change of reviews starts a
// new key, so a request that began before the change does not answer for after it.
const inFlight = new Map<string, Promise<Page>>();

export function firstReviewsPage(canonicalGameId: string, sort: ReviewSort, filter: ReviewFilter): Promise<Page> {
  const key = [get(reviewsChanged), canonicalGameId, sort, filter].join('\n');
  const held = inFlight.get(key);
  if (held) return held;
  const request = list(canonicalGameId, sort, filter, '').finally(() => inFlight.delete(key));
  inFlight.set(key, request);
  return request;
}
