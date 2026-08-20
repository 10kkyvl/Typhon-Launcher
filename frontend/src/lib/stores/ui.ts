import { writable } from 'svelte/store';

export const libraryView = writable<'grid' | 'list'>('grid');
export const installedView = writable<'list' | 'grid'>('list');

const SCALE_KEY = 'aurora.ui-scale';

function initialScale() {
  const stored = Number(localStorage.getItem(SCALE_KEY));
  return stored >= 0.9 && stored <= 1.25 ? stored : 1;
}

export const uiScale = writable<number>(initialScale());

uiScale.subscribe((value) => {
  document.documentElement.style.setProperty('--ui-scale', String(value));
  localStorage.setItem(SCALE_KEY, String(value));
});
