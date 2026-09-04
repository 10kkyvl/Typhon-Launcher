import { writable } from 'svelte/store';

export const libraryView = writable<'grid' | 'list'>('grid');
export const catalogView = writable<'grid' | 'list'>('grid');
export const installedView = writable<'list' | 'grid'>('list');
export const friendsView = writable<'list' | 'grid'>('list');
