import { writable } from 'svelte/store';
import { storedValue } from './libraryPrefs';

export const libraryView = storedValue<'grid' | 'list'>('localStorage', 'typhon:library:view', (raw) => (raw === 'list' ? 'list' : 'grid'));
export const catalogView = writable<'grid' | 'list'>('grid');
export const installedView = writable<'list' | 'grid'>('list');
export const friendsView = writable<'list' | 'grid'>('list');
