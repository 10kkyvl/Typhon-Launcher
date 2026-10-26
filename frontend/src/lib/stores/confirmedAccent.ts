import { get, writable } from 'svelte/store';
import { displayedAccent } from '../theme/apply';

export const confirmedAccent = writable('');

export function confirmAccent(accent = ''): void {
  confirmedAccent.set(accent || get(displayedAccent));
}
