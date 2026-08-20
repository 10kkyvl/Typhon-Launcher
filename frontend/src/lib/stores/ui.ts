import { writable } from 'svelte/store';

export const libraryView = writable<'grid' | 'list'>('grid');
export const installedView = writable<'list' | 'grid'>('list');
