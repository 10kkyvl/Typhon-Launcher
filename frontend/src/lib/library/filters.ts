export type LibraryFilter = 'all' | 'installed' | 'recent' | 'hidden';

export function nextLibraryFilter(current: LibraryFilter, selected: LibraryFilter): LibraryFilter {
  return selected !== 'all' && selected === current ? 'all' : selected;
}
