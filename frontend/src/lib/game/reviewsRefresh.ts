import { writable } from 'svelte/store';

export const reviewsChanged = writable(0);

export function notifyReviewsChanged(): void {
  reviewsChanged.update((n) => n + 1);
}
